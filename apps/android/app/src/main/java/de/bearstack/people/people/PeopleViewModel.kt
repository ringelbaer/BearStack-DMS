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
import android.util.Log
import de.bearstack.people.BuildConfig
import kotlinx.coroutines.*
import kotlinx.coroutines.flow.*
import org.json.JSONObject
import okhttp3.OkHttpClient

data class PeopleState(
    val connected: Boolean = false, val busy: Boolean = false, val person: Person? = null,
    val error: String? = null, val certificate: CertificateOffer? = null,
    val naming: Boolean = false, val name: String = "", val suggestions: List<Person> = emptyList(),
    val duplicates: List<Person> = emptyList(), val undoIgnores: List<Long> = emptyList(), val unresolved: Boolean = false,
    val stats: List<Statistics> = emptyList(), val skipped: Int = 0, val canGoBack: Boolean = false,
    val directory: Boolean = false, val namedPeople: List<Person> = emptyList(),
    val namedCursor: Long = 0, val namedUpper: Long = 0, val namedHasNext: Boolean = false,
    val selectedPerson: Person? = null, val namedQuery: String = "", val loadedNamedQuery: String = "",
    val namedSearch: Boolean = false, val removeFace: Long? = null, val removeRevision: Long = 0,
    val mergeReview: Boolean = false, val mergeSuggestion: MergeSuggestion? = null,
)
class PeopleViewModel private constructor(application: Application, private val db: LabelingDatabase,
    initialRepository: PeopleRepository?) : AndroidViewModel(application) {
    constructor(application: Application) : this(application, LabelingDatabase.open(application), null)
    internal constructor(application: Application, database: LabelingDatabase) : this(application,database,null)
    internal constructor(application: Application, database: LabelingDatabase, service: LabelingService, session: Session) :
        this(application, database, PeopleRepository(database, service, session))
    private val originalPreloader = WifiOriginalPreloader(application, viewModelScope)
    private val store = ProfileStore(application)
    private val mutable = MutableStateFlow(PeopleState())
    val state = mutable.asStateFlow()
    private var repository: PeopleRepository? = null
    private var api: LabelingApi? = null
    var images: ImageLoader? = null; private set
    private var profileToConfirm: Profile? = null
    private val ignoreJobs = mutableMapOf<Long,Job>()
    private val undoRequests = mutableSetOf<Long>()
    private var inBackground = false
    private var search: Job? = null
    private var directorySearch: Job? = null
    private var statsJob: Job? = null
    private val preloads = mutableListOf<coil.request.Disposable>()

    init { task {
        if (initialRepository == null) store.read()?.let { open(it, false) }
        else {
            repository=initialRepository
            update { it.copy(connected=true) }
            collectStatistics()
            initialRepository.resolve()
            initialRepository.restoreIgnores()
            loadNext()
        }
    } }
    private fun update(block: (PeopleState) -> PeopleState) = mutable.update(block)
    private fun task(clearError: Boolean = true, block: suspend () -> Unit) {
        if (state.value.busy) return
        update { it.copy(busy=true, error=if(clearError) null else it.error) }
        viewModelScope.launch {
            try { block() }
            catch (e: CancellationException) { throw e }
            catch (e: Exception) {
                if (e is ApiFailure && e.status == 409) {
                    if (e.code == "name_exists") {
                        runCatching { repository?.api?.suggestions(state.value.name, true) }.getOrNull()?.let { names ->
                            update { it.copy(naming=true,duplicates=names.filterNot { p -> it.directory && p.id==it.selectedPerson?.id }) }
                        }
                    } else {
                        // A changed source or target always requires a new explicit decision.
                        update { it.copy(naming=false,duplicates=emptyList(),suggestions=emptyList(),removeFace=null) }
                        runCatching {
                            val repo = repository ?: return@runCatching
                            if (repo.api.session().scope != repo.scope) {
                                clearConnection(); update { it.copy(error="Der Datenbestand wurde geändert. Bitte neu verbinden.") }
                            } else if(state.value.mergeReview) loadMergeSuggestion()
                            else if(state.value.directory) refreshSelectedPerson() else loadNext()
                        }
                    }
                }
                val pending = runCatching { repository?.pending() != null }.getOrDefault(false)
                update { it.copy(error=message(e,pending), unresolved=pending) }
            } finally { update { it.copy(busy=false) } }
        }
    }
    private fun message(e: Exception, pending: Boolean = false): String {
        connectionDiagnostic(e,pending)?.let { diagnostic ->
            if (BuildConfig.DEBUG) Log.w("BearStackConnection", "stage=${diagnostic.stage} code=${diagnostic.code}")
            return diagnostic.text
        }
        return if(e is ApiFailure) e.message.orEmpty() else e.message ?: "Die Aktion konnte nicht abgeschlossen werden."
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
        val session = try { remote.session() } catch (e: Exception) {
            // Cleanup must not replace a useful 401/403 error with a socket-close error.
            runCatching { Connections.close(client) }
            if(e is java.io.IOException && e !is ApiFailure) throw ConnectionAttemptException(ConnectionStage.SIGN_IN,e)
            throw e
        }
        if (save) store.write(profile)
        clearConnection()
        api = remote
        images = ImageLoader.Builder(getApplication()).okHttpClient(client).diskCachePolicy(CachePolicy.DISABLED)
            .memoryCache { OriginalMemoryCache(MemoryCache.Builder(getApplication()).maxSizeBytes(16 * 1024 * 1024)
                .weakReferencesEnabled(false).build()) }.build()
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
        repository!!.restoreIgnores()
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
        val queue=repo.state()
        update { it.copy(person=null,skipped=queue.skipped.ids().size,canGoBack=queue.skipHistory.isNotEmpty()) }
        val person = repo.next()
        update { it.copy(person=person) }
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
    fun original(face: Long) = api?.original(face)
    fun originalKey(face: Long): String? {
        val current=state.value
        val person=if(current.mergeReview) current.mergeSuggestion?.let {
            if(face in it.source.faces) it.source else it.target
        } else if(current.directory) current.selectedPerson else current.person
        return person?.originalKeys?.get(face)?.let { "${repository?.scope}:$it" }
    }
    suspend fun prefetchOriginals(faces: List<Long>) {
        val loader = images ?: return
        val remote = api ?: return
        val requests = faces.associate { face ->
            val url = remote.original(face)
            (originalKey(face) ?: url) to de.bearstack.people.ui.originalPhotoRequest(getApplication(),url,originalKey(face))
        }
        originalPreloader.preload(requests.keys.toList()) { key ->
            if (images === loader) loader.execute(requests.getValue(key))
        }
    }
    fun gallery(name: String) = api?.gallery(name)
    private fun editable() = state.value.connected && !state.value.busy && !state.value.unresolved
    fun openMergeReview() { if(editable() && !state.value.naming) task {
        search?.cancel(); searchPreload?.cancel()
        preloads.forEach { it.dispose() }; preloads.clear()
        update { it.copy(mergeReview=true,mergeSuggestion=null) }
        loadMergeSuggestion()
    } }
    private suspend fun loadMergeSuggestion() {
        update { it.copy(mergeSuggestion=null) }
        val repo=repository ?: return
        val session=repo.api.session()
        require(session.scope==repo.scope) { "Der Datenbestand wurde geändert. Bitte neu verbinden." }
        if(!session.mergeSuggestions) throw ApiFailure(404,"not_found","Ähnliche Gruppen benötigen BearStack 0.49.0.")
        val suggestion=repo.api.nextMergeSuggestion()
        update { it.copy(mergeSuggestion=suggestion) }
    }
    fun decideMerge(accept: Boolean) { if(editable() && state.value.mergeReview) task {
        val suggestion=state.value.mergeSuggestion ?: return@task
        val repo=repository ?: return@task
        repo.prepare(suggestion.source,if(accept) "accept_merge" else "reject_merge",
            target=suggestion.target,suggestionId=suggestion.id)
        repo.resolve()
        loadMergeSuggestion()
    } }
    fun closeMergeReview() { if(editable()) task {
        update { it.copy(mergeReview=false,mergeSuggestion=null) }
        loadNext()
    } }
    fun page(delta: Int) { if (editable()) task {
        val old = state.value.person ?: return@task
        val offset = (old.offset + delta * 4).coerceAtLeast(0)
        val p = repository!!.page(offset)
        update { it.copy(person=p) }; preload(p)
        if (p.revision != old.revision || p.name.isNotEmpty()) { loadNext(); update { it.copy(error="Die Gruppe wurde geändert. Bitte erneut prüfen.") } }
    } }
    fun startNaming() { if(editable()) update { it.copy(naming=true,name=if(it.directory) it.selectedPerson?.name.orEmpty() else "",suggestions=emptyList(),duplicates=emptyList()) } }
    fun closeNaming() { if(editable()) { search?.cancel(); update { it.copy(naming=false,duplicates=emptyList()) } } }
    fun nameChanged(name: String) {
        if (!editable()) return
        update { it.copy(name=name,suggestions=emptyList(),duplicates=emptyList()) }
        search?.cancel()
        if (name.isBlank() || state.value.directory) return
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
                .filterNot { state.value.directory && it.id==state.value.selectedPerson?.id }
            if (duplicates.isNotEmpty()) { update { it.copy(duplicates=duplicates) }; return@task }
        }
        if(state.value.directory) manage("rename",name=state.value.name.trim(),allowDuplicate=allowDuplicate)
        else mutate("name",name=state.value.name.trim(),allowDuplicate=allowDuplicate)
    } }
    fun assign(target: Person) { if(editable()) task { search?.cancel(); mutate("assign",target=target) } }
    fun detach(face: Long) { if(editable()) task { mutate("detach",face=face) } }
    fun openDirectory() { if(editable() && !state.value.naming) task {
        update { it.copy(directory=true,selectedPerson=null,removeFace=null,namedPeople=emptyList(),namedHasNext=false) }
        loadNamedPeople(true)
    } }
    fun namedQueryChanged(query: String) {
        if(!state.value.directory || state.value.selectedPerson!=null || state.value.unresolved) return
        update {it.copy(namedQuery=query.take(200),namedPeople=emptyList(),namedHasNext=false,error=null)}
        directorySearch?.cancel()
        val repo=repository
        directorySearch=viewModelScope.launch {
            delay(250)
            state.first {!it.busy}
            if(repository===repo && state.value.directory && state.value.selectedPerson==null) task {loadNamedPeople(true)}
        }
    }
    fun moreNamedPeople() { if(editable() && state.value.namedHasNext && state.value.namedQuery==state.value.loadedNamedQuery) task { loadNamedPeople(false) } }
    private suspend fun loadNamedPeople(reset: Boolean) {
        val repo=repository ?: return
        val query=state.value.namedQuery
        try {
            val upper=if(reset) {
                val fresh=repo.api.session()
                require(fresh.scope==repo.scope) { "Der Datenbestand wurde geändert. Bitte neu verbinden." }
                require(fresh.namedPeople) { "Der Personenbereich benötigt BearStack 0.43.0 oder neuer." }
                update {it.copy(namedSearch=fresh.namedSearch)}
                require(query.isBlank() || fresh.namedSearch) {"Die Textsuche benötigt BearStack 0.45.0 oder neuer."}
                fresh.upper
            } else state.value.namedUpper
            val page=repo.api.searchPeople(if(reset) 0 else state.value.namedCursor,upper,query.trim())
            if(state.value.namedQuery==query && state.value.directory && state.value.selectedPerson==null) update {
                it.copy(namedPeople=if(reset) page.people else (it.namedPeople+page.people).distinctBy { p -> p.id },
                    namedCursor=page.next,namedUpper=upper,namedHasNext=page.hasNext,loadedNamedQuery=query)
            }
        } catch(e: CancellationException) {throw e}
        catch(e: Exception) {if(state.value.namedQuery==query) throw e}
    }
    fun openPerson(person: Person) { if(editable()) task {
        directorySearch?.cancel()
        update { it.copy(selectedPerson=person.copy(faces=emptyList(),offset=0),removeFace=null) }
        refreshSelectedPerson()
    } }
    private fun setSelectedPerson(person: Person?) {
        val old=state.value.selectedPerson ?: return
        update {it.copy(selectedPerson=person,removeFace=null,namedPeople=it.namedPeople.mapNotNull { p ->
            if(p.id!=old.id) p else person?.copy(faces=emptyList(),facePaths=emptyMap(),faceBounds=emptyMap(),originalKeys=emptyMap(),favorites=emptySet())
        })}
    }
    private suspend fun refreshSelectedPerson() {
        val old=state.value.selectedPerson ?: return
        val remote=repository?.api ?: return
        val person=try { remote.personFaces(old.id,0,0) }
            catch(e: ApiFailure) { if(e.status!=404) throw e; null }
        setSelectedPerson(person?.takeIf {it.name.isNotEmpty() && it.count>0}?.copy(offset=0))
    }
    fun morePersonFaces() {
        val old=state.value.selectedPerson ?: return
        if(!editable() || state.value.naming || state.value.removeFace!=null || old.faces.size>=old.count) return
        task {
            val page=repository!!.api.personFaces(old.id,old.faces.size,old.faces.lastOrNull() ?: 0)
            if(page.revision!=old.revision || page.name!=old.name || page.count!=old.count) {
                refreshSelectedPerson()
                update {it.copy(error="Die Person wurde geändert. Bitte erneut prüfen.")}
            } else {
                val faces=(old.faces+page.faces).distinct()
                require(faces.size>old.faces.size) {"Keine weiteren Bilder geladen. Bitte erneut versuchen."}
                update {it.copy(selectedPerson=old.copy(faces=faces,facePaths=old.facePaths+page.facePaths,
                    faceBounds=old.faceBounds+page.faceBounds,originalKeys=old.originalKeys+page.originalKeys,
                    favorites=old.favorites+page.favorites))}
            }
        }
    }
    fun closePerson() { if(editable() && !state.value.naming) {
        update { it.copy(selectedPerson=null,removeFace=null,error=null) }
        if(state.value.namedQuery.isNotBlank()) task {loadNamedPeople(true)}
    } }
    fun closeDirectory() { if(editable() && !state.value.naming) task {
        directorySearch?.cancel()
        update { it.copy(directory=false,selectedPerson=null,removeFace=null,error=null) }
        loadNext()
    } }
    fun requestUnassign(face: Long) {
        val person=state.value.selectedPerson ?: return
        if(editable() && !state.value.naming && face in person.faces) update {it.copy(removeFace=face,removeRevision=person.revision)}
    }
    fun cancelUnassign() { if(!state.value.busy) update {it.copy(removeFace=null)} }
    fun confirmUnassign() {
        val current=state.value
        val face=current.removeFace ?: return
        if(!editable()) return
        update {it.copy(removeFace=null)}
        if(current.selectedPerson?.revision==current.removeRevision) unassign(face)
        else update {it.copy(error="Die Person wurde geändert. Bitte erneut prüfen.")}
    }
    fun unassign(face: Long) { if(editable()) task { manage("unassign",face=face) } }
    fun favorite(face: Long) { if(editable()) task {
        val person=state.value.selectedPerson ?: return@task
        manage("favorite",face=face,favorite=face !in person.favorites)
    } }
    private suspend fun applyManagementReceipt(receipt: Receipt, body: JSONObject) {
        val old=state.value.selectedPerson ?: return
        if(receipt.source!=old.id || receipt.action !in listOf("rename","favorite","unassign")) return
        if(receipt.sourceRevision<=0) {refreshSelectedPerson();return}
        val face=body.optLong("face_id")
        var person=old.copy(revision=receipt.sourceRevision)
        when(receipt.action) {
            "rename" -> person=person.copy(name=body.getString("name").trim().replace(Regex("\\s+")," "))
            "favorite" -> person=person.copy(favorites=if(body.getBoolean("favorite")) person.favorites+face else person.favorites-face)
            "unassign" -> person=person.copy(count=person.count-1,faces=person.faces-face,favorites=person.favorites-face,
                facePaths=person.facePaths-face,faceBounds=person.faceBounds-face,originalKeys=person.originalKeys-face,
                faceId=person.faces.firstOrNull {it!=face} ?: 0)
        }
        setSelectedPerson(person.takeIf {it.count>0})
    }
    private suspend fun manage(action: String, face: Long = 0, name: String = "", allowDuplicate: Boolean = false,
        favorite: Boolean? = null) {
        val person=state.value.selectedPerson ?: return
        val repo=repository ?: return
        repo.prepare(person,action,name=name,face=face,allowDuplicate=allowDuplicate,favorite=favorite)
        val body=JSONObject(repo.pending()!!.body)
        update { it.copy(unresolved=true) }
        val receipt=repo.resolve()!!
        update { it.copy(unresolved=false,naming=false,duplicates=emptyList()) }
        applyManagementReceipt(receipt,body)
    }
    private suspend fun mutate(action: String, name: String = "", target: Person? = null, face: Long = 0, allowDuplicate: Boolean = false) {
        val p = state.value.person ?: return
        val repo = repository ?: return
        repo.prepare(p,action,name,target,face,allowDuplicate)
        update { it.copy(unresolved=true) }
        repo.resolve()
        update { it.copy(unresolved=false,naming=false,duplicates=emptyList(),error=null) }
        loadNext()
    }
    private suspend fun awaitReady(repo: PeopleRepository, waitForNaming: Boolean = false): Boolean {
        fun ready(value: PeopleState) = !value.busy && !value.unresolved && (!waitForNaming || (!value.naming && value.removeFace==null))
        while(repository===repo) {
            state.first { ready(it) }
            // Several expired timers may wake together. Recheck the live state
            // before claiming the single writer; none may silently be dropped.
            if(repository===repo && ready(state.value)) return true
        }
        return false
    }
    private fun whenReady(repo: PeopleRepository, block: suspend () -> Unit): Job = viewModelScope.launch {
        if(awaitReady(repo)) task(block=block)
    }
    fun ignore() { if(editable() && state.value.person!=null) task {
        val person=state.value.person ?: return@task
        val repo=repository ?: return@task
        search?.cancel()
        repo.stageIgnore(person)
        update {it.copy(naming=false,error=null)}
        if(inBackground) repo.restoreIgnores(setOf(person.id))
        else {
            update {it.copy(undoIgnores=it.undoIgnores+person.id)}
            ignoreJobs[person.id]=viewModelScope.launch {
                delay(5000)
                update {it.copy(undoIgnores=it.undoIgnores-person.id)}
                // An expired toast must not disable the open name field and end its IME session.
                // Keep the five-second undo deadline, but claim the writer only after naming closes.
                if(awaitReady(repo,waitForNaming=true)) task(clearError=false) {
                    ignoreJobs.remove(person.id)
                    if(inBackground) { repo.restoreIgnores(setOf(person.id)); return@task }
                    // Commit the captured group, never the card now on screen.
                    repo.prepare(person,"ignore")
                    try { repo.resolve() }
                    catch(e: ApiFailure) {
                        if(e.status!=400 && e.status!=404 && e.status!=409) throw e
                        repo.restoreIgnores(setOf(person.id))
                        update {it.copy(error="Ignorieren wurde nicht gespeichert. Die Gruppe wurde zur erneuten Prüfung vorgemerkt.")}
                    }
                }
            }
        }
        loadNext()
    } }
    fun undoIgnore() {
        val id=state.value.undoIgnores.lastOrNull() ?: return
        val repo=repository ?: return
        ignoreJobs.remove(id)?.cancel()
        undoRequests+=id
        update {it.copy(undoIgnores=it.undoIgnores-id)}
        whenReady(repo) {
            try { repo.restoreIgnores(setOf(id),show=id) } finally { undoRequests-=id }
            search?.cancel()
            update {it.copy(naming=false,suggestions=emptyList(),duplicates=emptyList(),directory=false,selectedPerson=null,mergeReview=false,mergeSuggestion=null)}
            loadNext()
        }
    }
    fun foreground() { inBackground=false }
    fun background() {
        inBackground=true
        val repo=repository ?: return
        val ids=ignoreJobs.keys.toSet()
        ignoreJobs.values.forEach {it.cancel()};ignoreJobs.clear()
        update {it.copy(undoIgnores=emptyList())}
        if(ids.isNotEmpty()) whenReady(repo) {
            repo.restoreIgnores(ids)
            if(state.value.person==null) loadNext()
        }
    }
    fun skip() { if(editable()) task { repository!!.skip(state.value.person ?: return@task); loadNext() } }
    fun back() { if(editable() && !state.value.naming && state.value.canGoBack) task {
        val repo=repository ?: return@task
        val person=repo.back()
        val queue=repo.state()
        update { it.copy(person=person ?: it.person,skipped=queue.skipped.ids().size,
            canGoBack=queue.skipHistory.isNotEmpty(),
            error=if(person==null) "Die übersprungenen Gruppen wurden inzwischen bearbeitet oder sind nicht mehr verfügbar." else null) }
        if(person!=null) preload(person)
    } }
    fun retry() = task {
        val body=repository?.pending()?.body?.let {JSONObject(it)}
        val receipt = repository?.resolve()
        repository?.let { repo ->
            repo.restoreIgnores(repo.state().stagedIgnores.positions().map {it.id}.toSet()-ignoreJobs.keys-undoRequests)
        }
        update { it.copy(unresolved=false,naming=if(receipt!=null && (receipt.source==it.person?.id || receipt.source==it.selectedPerson?.id)) false else it.naming) }
        if(state.value.mergeReview) loadMergeSuggestion()
        else if(state.value.directory) {
            if(receipt!=null && body!=null && state.value.selectedPerson!=null) applyManagementReceipt(receipt,body)
            else if(state.value.selectedPerson!=null) refreshSelectedPerson() else loadNamedPeople(true)
        } else loadNext()
    }
    fun newPass(skipped: Boolean) { if(editable()) task { repository!!.newPass(skipped); loadNext() } }
    fun switchConnection() { if(!state.value.busy) task { repository?.restoreIgnores(); clearConnection(); store.clear() } }
    private fun detachConnection(): Pair<ImageLoader?,OkHttpClient?> {
        ignoreJobs.values.forEach {it.cancel()};ignoreJobs.clear()
        undoRequests.clear()
        search?.cancel(); directorySearch?.cancel(); searchPreload?.cancel(); statsJob?.cancel()
        preloads.forEach { it.dispose() }; preloads.clear()
        val resources=images to api?.client
        images=null;api=null;repository=null
        update { PeopleState(busy=it.busy) }
        return resources
    }
    private suspend fun closeConnection(resources: Pair<ImageLoader?,OkHttpClient?>) = withContext(NonCancellable + Dispatchers.IO) {
        try { resources.first?.memoryCache?.clear(); resources.first?.shutdown() }
        finally { resources.second?.let {Connections.close(it)} }
    }
    private suspend fun clearConnection() {
        closeConnection(detachConnection())
    }
    override fun onCleared() {
        val resources=detachConnection()
        // viewModelScope is already cancelled here; resource cleanup must finish independently.
        CoroutineScope(Dispatchers.IO).launch {
            try { closeConnection(resources) } finally { db.close() }
        }
        super.onCleared()
    }
}
