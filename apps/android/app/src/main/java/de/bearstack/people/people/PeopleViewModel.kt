package de.bearstack.people.people

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import coil.ImageLoader
import coil.memory.MemoryCache
import coil.request.CachePolicy
import coil.request.ImageRequest
import de.bearstack.people.connection.*
import de.bearstack.people.data.local.*
import de.bearstack.people.data.remote.*
import java.time.LocalDate
import java.time.ZoneId
import javax.net.ssl.SSLException
import kotlinx.coroutines.*
import kotlinx.coroutines.flow.*
import org.json.JSONObject

data class PeopleState(
    val connected: Boolean = false, val busy: Boolean = false, val person: Person? = null,
    val error: String? = null, val certificate: CertificateOffer? = null,
    val naming: Boolean = false, val name: String = "", val suggestions: List<Person> = emptyList(),
    val duplicates: List<Person> = emptyList(), val undoSeconds: Int = 0, val unresolved: Boolean = false,
    val stats: List<Statistics> = emptyList(), val skipped: Int = 0,
)
class PeopleViewModel private constructor(application: Application, private val db: LabelingDatabase,
    initialRepository: PeopleRepository?) : AndroidViewModel(application) {
    constructor(application: Application) : this(application, LabelingDatabase.open(application), null)
    internal constructor(application: Application, database: LabelingDatabase, service: LabelingService, session: Session) :
        this(application, database, PeopleRepository(database, service, session))
    private val store = ProfileStore(application)
    private val mutable = MutableStateFlow(PeopleState())
    val state = mutable.asStateFlow()
    private var repository: PeopleRepository? = null
    private var api: LabelingApi? = null
    var images: ImageLoader? = null; private set
    private var profileToConfirm: Profile? = null
    private var undo: Job? = null
    private var search: Job? = null
    private var statsJob: Job? = null
    private val preloads = mutableListOf<coil.request.Disposable>()

    init { task {
        if (initialRepository == null) store.read()?.let { open(it, false) }
        else {
            repository=initialRepository
            update { it.copy(connected=true) }
            collectStatistics()
            initialRepository.resolve()
            loadNext()
        }
    } }
    private fun update(block: (PeopleState) -> PeopleState) = mutable.update(block)
    private fun task(block: suspend () -> Unit) {
        if (state.value.busy) return
        update { it.copy(busy=true, error=null) }
        viewModelScope.launch {
            try { block() }
            catch (e: CancellationException) { throw e }
            catch (e: Exception) {
                if (e is ApiFailure && e.status == 409) {
                    if (e.code == "name_exists") {
                        runCatching { repository?.api?.suggestions(state.value.name, true) }.getOrNull()?.let { names ->
                            update { it.copy(naming=true,duplicates=names) }
                        }
                    } else {
                        // A changed source or target always requires a new explicit decision.
                        update { it.copy(naming=false,duplicates=emptyList(),suggestions=emptyList()) }
                        runCatching {
                            val repo = repository ?: return@runCatching
                            if (repo.api.session().scope != repo.scope) {
                                clearConnection(); update { it.copy(error="Der Datenbestand wurde geändert. Bitte neu verbinden.") }
                            } else loadNext()
                        }
                    }
                }
                val pending = runCatching { repository?.pending() != null }.getOrDefault(false)
                update { it.copy(error=message(e), unresolved=pending, undoSeconds=0) }
            } finally { update { it.copy(busy=false) } }
        }
    }
    private fun message(e: Exception): String = when(e) {
        is SSLException -> "Zertifikatsprüfung fehlgeschlagen. Adresse, Gültigkeit und Fingerabdruck erneut prüfen."
        is ApiFailure -> e.message.orEmpty()
        is java.io.IOException -> "Verbindung unterbrochen. Erneut versuchen; eine offene Aktion wird zuerst geprüft."
        else -> e.message ?: "Die Aktion konnte nicht abgeschlossen werden."
    }
    fun connect(url: String, username: String, password: String) = task {
        val profile = Profile(Connections.address(url).toString(),username.trim(),password)
        val certificate = Connections.inspect(profile.url)
        if (certificate != null) {
            profileToConfirm = profile
            update { it.copy(certificate=certificate) }
        } else open(profile, true)
    }
    fun confirmCertificate() = task {
        val profile = profileToConfirm ?: return@task
        val certificate = state.value.certificate ?: return@task
        open(profile.copy(certificate=certificate.encoded),true)
        profileToConfirm = null
        update { it.copy(certificate=null) }
    }
    fun cancelCertificate() { profileToConfirm=null; update { it.copy(certificate=null) } }
    private suspend fun open(profile: Profile, save: Boolean) {
        val client = Connections.client(profile)
        val remote = LabelingApi(client, profile.url)
        val session = try { remote.session() } catch (e: Exception) { client.connectionPool.evictAll(); client.dispatcher.executorService.shutdown(); throw e }
        if (save) store.write(profile)
        clearConnection()
        api = remote
        images = ImageLoader.Builder(getApplication()).okHttpClient(client).diskCachePolicy(CachePolicy.DISABLED)
            .memoryCache { MemoryCache.Builder(getApplication()).maxSizeBytes(16 * 1024 * 1024).build() }.build()
        repository = PeopleRepository(db,remote,session)
        update { it.copy(connected=true,certificate=null) }
        collectStatistics()
        val pending = repository!!.pending()
        if (pending != null) {
            val body = JSONObject(pending.body)
            update { it.copy(name=body.optString("name"),naming=body.optString("action") == "name",unresolved=true) }
            repository!!.resolve()
            update { it.copy(unresolved=false,naming=false) }
        }
        loadNext()
    }
    @OptIn(ExperimentalCoroutinesApi::class)
    private fun collectStatistics() {
        statsJob?.cancel()
        val repo = repository ?: return
        statsJob = viewModelScope.launch {
            flow { while(true) { emit(LocalDate.now().atStartOfDay(ZoneId.systemDefault()).toEpochSecond()); delay(60_000) } }
                .distinctUntilChanged().flatMapLatest { repo.statistics(it) }.collect { stats -> update { it.copy(stats=stats) } }
        }
    }
    private suspend fun loadNext() {
        val repo = repository ?: return
        // The queue may already have advanced after a confirmed mutation or local skip.
        // Never leave its previous card actionable when loading the next one fails.
        update { it.copy(person=null) }
        val person = repo.next()
        val skipped = repo.state().skipped.ids().size
        update { it.copy(person=person,skipped=skipped) }
        preload(person)
    }
    private fun preload(person: Person?) {
        preloads.forEach { it.dispose() }; preloads.clear()
        searchPreload?.cancel()
        if (person == null || person.offset + 4 >= person.count) return
        val remote = api ?: return
        // Metadata for at most one next four-face page. Never prefetch enlarged images.
        searchPreload = viewModelScope.launch {
            try {
                val next = remote.person(person.id,person.offset+4)
                if (state.value.person?.let { it.id == person.id && it.revision == next.revision && it.offset == person.offset } == true) {
                    next.faces.take(4).forEach { face ->
                        images?.enqueue(ImageRequest.Builder(getApplication()).data(remote.image(face)).size(160).build())?.let { preloads += it }
                    }
                }
            } catch (_: Exception) { /* Prefetch failure never affects an explicit action. */ }
        }
    }
    private var searchPreload: Job? = null
    fun image(face: Long, large: Boolean = false) = api?.image(face,large)
    private fun editable() = state.value.connected && !state.value.busy && !state.value.unresolved && state.value.undoSeconds == 0
    fun page(delta: Int) { if (editable()) task {
        val old = state.value.person ?: return@task
        val offset = (old.offset + delta * 4).coerceAtLeast(0)
        val p = repository!!.page(offset)
        update { it.copy(person=p) }; preload(p)
        if (p.revision != old.revision || p.name.isNotEmpty()) { loadNext(); update { it.copy(error="Die Gruppe wurde geändert. Bitte erneut prüfen.") } }
    } }
    fun startNaming() { if(editable()) update { it.copy(naming=true,name="",suggestions=emptyList(),duplicates=emptyList()) } }
    fun closeNaming() { if(editable()) { search?.cancel(); update { it.copy(naming=false,duplicates=emptyList()) } } }
    fun nameChanged(name: String) {
        if (!editable()) return
        update { it.copy(name=name,suggestions=emptyList(),duplicates=emptyList()) }
        search?.cancel()
        if (name.isBlank()) return
        search = viewModelScope.launch {
            delay(250)
            try {
                val results = repository!!.api.suggestions(name)
                update { if(it.name == name) it.copy(suggestions=results) else it }
            } catch (e: CancellationException) { throw e }
            catch (e: Exception) { update { it.copy(error=message(e)) } }
        }
    }
    fun submitName(allowDuplicate: Boolean = false) { if(editable() && state.value.name.isNotBlank()) task {
        search?.cancel()
        if (!allowDuplicate) {
            val duplicates = repository!!.api.suggestions(state.value.name.trim(),true)
            if (duplicates.isNotEmpty()) { update { it.copy(duplicates=duplicates) }; return@task }
        }
        mutate("name",name=state.value.name.trim(),allowDuplicate=allowDuplicate)
    } }
    fun assign(target: Person) { if(editable()) task { search?.cancel(); mutate("assign",target=target) } }
    fun detach(face: Long) { if(editable()) task { mutate("detach",face=face) } }
    private suspend fun mutate(action: String, name: String = "", target: Person? = null, face: Long = 0, allowDuplicate: Boolean = false) {
        val p = state.value.person ?: return
        val repo = repository ?: return
        repo.prepare(p,action,name,target,face,allowDuplicate)
        update { it.copy(unresolved=true) }
        repo.resolve()
        update { it.copy(unresolved=false,naming=false,duplicates=emptyList(),error=null) }
        loadNext()
    }
    fun ignore() {
        if(!editable() || state.value.person == null) return
        search?.cancel()
        update { it.copy(undoSeconds=5,error=null,naming=false) }
        undo = viewModelScope.launch {
            for (remaining in 5 downTo 1) { update { it.copy(undoSeconds=remaining) }; delay(1000) }
            update { it.copy(undoSeconds=0) }
            task { mutate("ignore") }
        }
    }
    fun undoIgnore() { undo?.cancel(); undo=null; update { it.copy(undoSeconds=0) } }
    fun background() {
        if(state.value.undoSeconds > 0) {
            undoIgnore()
            update { it.copy(error="Ignorieren wurde beim Wechsel in den Hintergrund zurückgenommen.") }
        }
    }
    fun skip() { if(editable()) task { repository!!.skip(state.value.person ?: return@task); loadNext() } }
    fun retry() = task {
        val receipt = repository?.resolve()
        update { it.copy(unresolved=false,naming=if(receipt!=null) false else it.naming) }
        loadNext()
    }
    fun newPass(skipped: Boolean) { if(editable()) task { repository!!.newPass(skipped); loadNext() } }
    fun switchConnection() { if(!state.value.busy && state.value.undoSeconds == 0) task { clearConnection(); store.clear() } }
    private fun clearConnection() {
        search?.cancel(); searchPreload?.cancel(); statsJob?.cancel()
        preloads.forEach { it.dispose() }; preloads.clear()
        images?.memoryCache?.clear(); images?.shutdown(); images=null
        api?.client?.dispatcher?.cancelAll(); api?.client?.connectionPool?.evictAll()
        api?.client?.dispatcher?.executorService?.shutdown()
        api=null;repository=null
        update { PeopleState(busy=it.busy) }
    }
    override fun onCleared() { clearConnection(); db.close(); super.onCleared() }
}
