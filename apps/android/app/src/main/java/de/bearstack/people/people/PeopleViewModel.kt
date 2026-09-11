package de.bearstack.people.people

import de.bearstack.people.media.*
import de.bearstack.people.text.*
import de.bearstack.people.R
import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import coil.ImageLoader
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

data class PeopleState(
    val connected: Boolean = false, val busy: Boolean = false, val person: Person? = null,
    val error: UiText? = null, val certificate: CertificateOffer? = null,
    val naming: Boolean = false, val name: String = "", val suggestions: List<Person> = emptyList(),
    val faceMatches: List<FaceMatch> = emptyList(), val faceSearching: Boolean = false, val faceSearchDone: Boolean = false,
    val duplicates: List<Person> = emptyList(), val undoIgnores: List<Long> = emptyList(), val unresolved: Boolean = false,
    val stats: List<Statistics> = emptyList(), val skipped: Int = 0, val canGoBack: Boolean = false,
    val directory: Boolean = false, val namedPeople: List<Person> = emptyList(),
    val namedCursor: Long = 0, val namedUpper: Long = 0, val namedHasNext: Boolean = false,
    val selectedPerson: Person? = null, val namedQuery: String = "", val loadedNamedQuery: String = "",
    val namedSearch: Boolean = false, val removeFace: Long? = null, val removeRevision: Long = 0,
    val mergeNaming: Boolean = false, val mergeReview: Boolean = false, val mergeSuggestion: MergeSuggestion? = null,
    val mergeSideActions: Boolean = false, val mergeNamingSide: Long? = null, val mergePendingSide: Long? = null,
    val mergeSideResults: Map<Long,UiText> = emptyMap(),
    val showGallery: Boolean = false, val canManagePeople: Boolean = false,
    val manualMerge: Boolean = false,
)
class PeopleViewModel private constructor(application: Application, private val database: Lazy<LabelingDatabase>,
    initialRepository: PeopleRepository?) : AndroidViewModel(application) {
    // A gallery-only session must not open the people queue's database.
    private val db by database
    constructor(application: Application) : this(application, lazy { LabelingDatabase.open(application) }, null)
    internal constructor(application: Application, database: LabelingDatabase) : this(application,lazyOf(database),null)
    internal constructor(application: Application, database: Lazy<LabelingDatabase>) : this(application,database,null)
    internal constructor(application: Application, database: LabelingDatabase, service: LabelingService, session: Session) :
        this(application, lazyOf(database), PeopleRepository(database, service, session))
    private val originalPreloader = WifiOriginalPreloader(application, viewModelScope)
    private val session = AppSession(application, viewModelScope) { detachPeople() }
    private val mutable = MutableStateFlow(PeopleState())
    val state = mutable.asStateFlow()
    private var repository: PeopleRepository? = null
    var manualMerges: ManualMergeController? = null
        private set
    private val api get() = session.active?.people
    val photos get() = session.active?.photos
    val images: ImageLoader? get() = session.active?.images
    private val ignoreJobs = mutableMapOf<Long,Job>()
    private val undoRequests = mutableSetOf<Long>()
    private var inBackground = false
    private var search: Job? = null
    private var faceSearch: Job? = null
    private var faceSearchGeneration = 0
    private var directorySearch: Job? = null
    private var statsJob: Job? = null
    private val preloads = mutableListOf<coil.request.Disposable>()
    private var excludedMergePair: Pair<Long,Long>? = null

    init { task {
        if (initialRepository == null) { if (session.restore()) attachPeople() }
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
                if (e is ApiFailure && (e.status == 409 || (e.status == 404 && state.value.mergePendingSide!=null))) {
                    if(state.value.manualMerge) {
                        if(e.code=="name_exists") manualMerges?.duplicateName() else manualMerges?.refresh()
                    } else if (e.code == "name_exists") {
                        runCatching { repository?.api?.suggestions(state.value.name, true) }.getOrNull()?.let { names ->
                            update { it.copy(naming=true,duplicates=names.filterNot { p -> it.directory && !it.mergeReview && p.id==it.selectedPerson?.id }) }
                        }
                    } else {
                        // A changed source or target always requires a new explicit decision.
                        update { it.copy(naming=false,duplicates=emptyList(),suggestions=emptyList(),removeFace=null) }
                        runCatching {
                            val repo = repository ?: return@runCatching
                            if (repo.api.session().scope != repo.scope) {
                                clearConnection(); update { it.copy(error=UiText(R.string.error_scope_changed)) }
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
    private fun message(e: Exception, pending: Boolean = false): UiText {
        connectionDiagnostic(e,pending)?.let { diagnostic ->
            if (BuildConfig.DEBUG) Log.w("BearStackConnection", "stage=${diagnostic.stage} code=${diagnostic.code}")
        }
        return failureText(e,pending)
    }
    fun connect(url: String, username: String, password: String) = task {
        if (session.connect(url, username, password)) attachPeople()
        update { it.copy(certificate=session.certificate) }
    }
    fun confirmCertificate() = task {
        if (session.confirmCertificate()) attachPeople()
        update { it.copy(certificate=session.certificate) }
    }
    fun cancelCertificate() { session.cancelCertificate(); update { it.copy(certificate=null) } }
    private suspend fun attachPeople() {
        val active = session.active ?: return
        repository = active.peopleSession?.let { PeopleRepository(db, active.people, it) }
        update { it.copy(connected=true,certificate=null,showGallery=photos!=null,canManagePeople=repository!=null) }
        if (repository == null) return
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
        val person=if(current.manualMerge) manualMerges?.state?.value?.let {s ->
            (s.selected+s.pages.values.flatten()).firstOrNull {it.faceId==face}
        } else if(current.mergeReview) current.mergeSuggestion?.let {
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
    fun openManualMerge() { if(editable() && !state.value.naming) task {
        val repo=repository ?: return@task
        search?.cancel();cancelFaceSearch();searchPreload?.cancel()
        ignoreJobs.values.forEach {it.cancel()};ignoreJobs.clear()
        repo.restoreIgnores()
        manualMerges?.cancel()
        manualMerges=ManualMergeController(viewModelScope,repo.api,repo.scope)
        update {it.copy(manualMerge=true,undoIgnores=emptyList())}
        manualMerges!!.reset()
    } }
    fun closeManualMerge() { if(editable() && manualMerges?.state?.value?.naming!=true) task {
        manualMerges?.cancel();manualMerges=null
        update {it.copy(manualMerge=false)}
        loadNext()
    } }
    fun combineManualGroups(named: Boolean = false) {
        val controller=manualMerges ?: return
        val selection=controller.state.value
        if(!editable() || !state.value.manualMerge || selection.selected.size<2 ||
            (!named && selection.naming) || (named && (!selection.naming || selection.name.isBlank()))) return
        controller.cancel()
        task {
            val repo=repository ?: return@task
            repo.prepareGroupMerge(selection.selected,if(named) selection.name.trim() else null,named && selection.duplicateName)
            update {it.copy(unresolved=true)}
            repo.resolve()
            update {it.copy(unresolved=false)}
            controller.refresh()
        }
    }
    private fun editable() = state.value.connected && !state.value.busy && !state.value.unresolved
    fun openMergeReview() { if(editable() && !state.value.naming) task {
        search?.cancel(); searchPreload?.cancel()
        preloads.forEach { it.dispose() }; preloads.clear()
        excludedMergePair=null
        update { it.copy(mergeReview=true,mergeSuggestion=null) }
        loadMergeSuggestion()
    } }
    private suspend fun loadMergeSuggestion() {
        update { it.copy(mergeSuggestion=null,naming=false,suggestions=emptyList(),duplicates=emptyList(),mergeNamingSide=null,mergePendingSide=null,mergeSideResults=emptyMap()) }
        val repo=repository ?: return
        val session=repo.api.session()
        requireMessage(session.scope==repo.scope,R.string.error_scope_changed)
        if(!session.mergeSuggestions) throw ApiFailure(404,"not_found",UiText(R.string.error_merge_version))
        val suggestion=repo.api.nextMergeSuggestion(excludedMergePair)
        update { it.copy(mergeSuggestion=suggestion,mergeNaming=session.mergeNaming,mergeSideActions=session.mergeSideActions) }
    }
    fun decideMerge(accept: Boolean) { if(editable() && state.value.mergeReview && !state.value.naming && state.value.mergeSideResults.isEmpty()) task {
        val suggestion=state.value.mergeSuggestion ?: return@task
        val repo=repository ?: return@task
        repo.prepare(suggestion.source,if(accept) "accept_merge" else "reject_merge",
            target=suggestion.target,suggestionId=suggestion.id)
        repo.resolve()
        loadMergeSuggestion()
    } }
    private fun mergeSide(id: Long): Person? = state.value.mergeSuggestion?.let {pair ->
        listOf(pair.source,pair.target).firstOrNull {it.id==id && it.name.isEmpty() && it.id !in state.value.mergeSideResults}
    }
    fun startMergeSideNaming(id: Long) {
        val person=mergeSide(id) ?: return
        if(editable() && state.value.mergeReview && state.value.mergeSideActions && !state.value.naming) {
            search?.cancel();cancelFaceSearch()
            update {it.copy(naming=true,mergeNamingSide=person.id,name="",suggestions=emptyList(),duplicates=emptyList(),error=null)}
        }
    }
    fun ignoreMergeSide(id: Long) {
        if(editable() && state.value.mergeReview && state.value.mergeSideActions && !state.value.naming && mergeSide(id)!=null) task {
            mutateMergeSide(id,"ignore")
        }
    }
    private suspend fun mutateMergeSide(id: Long, action: String, name: String="", target: Person?=null, allowDuplicate: Boolean=false) {
        val person=mergeSide(id) ?: return
        val repo=repository ?: return
        update {it.copy(mergePendingSide=id)}
        repo.prepare(person,action,name=name,target=target,allowDuplicate=allowDuplicate)
        val body=JSONObject(repo.pending()!!.body)
        val receipt=repo.resolve() ?: return
        completeMergeSide(receipt,body)
    }
    private fun completeMergeSide(receipt: Receipt, body: JSONObject) {
        val pair=state.value.mergeSuggestion ?: return
        requireMessage(receipt.source==state.value.mergePendingSide && receipt.action==body.getString("action"),R.string.error_receipt)
        val result=when(receipt.action) {
            "ignore" -> UiText(R.string.people_merge_side_ignored)
            "assign" -> UiText(R.string.people_merge_side_assigned)
            else -> UiText(R.string.people_merge_side_named,body.optString("name"))
        }
        val completed=mutableMapOf(receipt.source to result)
        if(receipt.target==pair.source.id || receipt.target==pair.target.id) completed[receipt.target]=UiText(R.string.people_merge_side_assigned)
        search?.cancel();cancelFaceSearch()
        update {it.copy(mergeSideResults=it.mergeSideResults+completed,mergePendingSide=null,mergeNamingSide=null,
            naming=false,suggestions=emptyList(),duplicates=emptyList(),unresolved=false)}
    }
    fun nextMerge() { if(editable() && !state.value.naming && state.value.mergeReview && state.value.mergeSideResults.isNotEmpty()) task {
        val pair=state.value.mergeSuggestion ?: return@task
        excludedMergePair=pair.source.id to pair.target.id
        loadMergeSuggestion()
    } }
    fun closeMergeReview() { if(editable() && !state.value.naming) task {
        update { it.copy(mergeReview=false,mergeSuggestion=null) }
        loadNext()
    } }
    fun page(delta: Int) { if (editable()) task {
        val old = state.value.person ?: return@task
        val offset = (old.offset + delta * 4).coerceAtLeast(0)
        val p = repository!!.page(offset)
        update { it.copy(person=p) }; preload(p)
        if (p.revision != old.revision || p.name.isNotEmpty()) { loadNext(); update { it.copy(error=UiText(R.string.error_group_changed)) } }
    } }
    fun startMergeNaming() {
        val s=state.value
        val pair=s.mergeSuggestion ?: return
        if(editable() && s.mergeReview && s.mergeNaming && s.mergeSideResults.isEmpty() && pair.source.name.isEmpty() && pair.target.name.isEmpty()) {
            search?.cancel(); cancelFaceSearch()
            update {it.copy(naming=true,mergeNamingSide=null,name="",suggestions=emptyList(),duplicates=emptyList(),error=null)}
        }
    }
    private suspend fun nameMerge(name: String = "", target: Person? = null, allowDuplicate: Boolean = false) {
        state.value.mergeNamingSide?.let {id ->
            if(state.value.naming && state.value.mergeSideActions) mutateMergeSide(id,if(target==null) "name" else "assign",name,target,allowDuplicate)
            return
        }
        val pair=state.value.mergeSuggestion ?: return
        if(!state.value.naming || !state.value.mergeNaming || pair.source.name.isNotEmpty() || pair.target.name.isNotEmpty()) return
        val repo=repository ?: return
        repo.prepare(pair.source,"name_merge",name=name,target=pair.target,allowDuplicate=allowDuplicate,
            suggestionId=pair.id,assignment=target)
        repo.resolve()
        loadMergeSuggestion()
    }
    fun startNaming() { if(editable()) { cancelFaceSearch(); update { it.copy(naming=true,name=if(it.directory) it.selectedPerson?.name.orEmpty() else "",suggestions=emptyList(),duplicates=emptyList()) } } }
    fun closeNaming() { if(editable()) { search?.cancel(); cancelFaceSearch(); update { it.copy(naming=false,mergeNamingSide=null,duplicates=emptyList()) } } }
    fun nameChanged(name: String) {
        if (!editable()) return
        cancelFaceSearch()
        update { it.copy(name=name,suggestions=emptyList(),duplicates=emptyList()) }
        search?.cancel()
        if (name.isBlank() || (state.value.directory && !state.value.mergeReview)) return
        search = viewModelScope.launch {
            delay(250)
            try {
                val results = repository!!.api.suggestions(name)
                ensureActive()
                update { if(it.naming && it.name == name && !it.faceSearching && !it.faceSearchDone) it.copy(suggestions=results) else it }
            } catch (e: CancellationException) { throw e }
            catch (e: Exception) { update { it.copy(error=message(e)) } }
        }
    }
    private fun cancelFaceSearch() {
        faceSearchGeneration++
        faceSearch?.cancel(); faceSearch=null
        update { it.copy(faceMatches=emptyList(),faceSearching=false,faceSearchDone=false) }
    }
    fun findFaceMatches() {
        val s=state.value
        if(!editable() || !s.naming || s.directory || s.faceSearching) return
        val person=(if(s.mergeReview) s.mergeNamingSide?.let {mergeSide(it)} ?: s.mergeSuggestion?.source else s.person) ?: return
        val face=person.faces.firstOrNull() ?: person.faceId
        if(face<=0) return
        val repo=repository ?: return
        search?.cancel(); cancelFaceSearch()
        val generation=faceSearchGeneration
        update { it.copy(faceSearching=true,suggestions=emptyList(),duplicates=emptyList(),error=null) }
        faceSearch=viewModelScope.launch {
            try {
                repo.api.faceMatches(face).collect { matches ->
                    if(repository===repo && generation==faceSearchGeneration && state.value.naming) update {
                        it.copy(faceMatches=matches.filterNot { match -> match.id==person.id })
                    }
                }
                if(repository===repo && generation==faceSearchGeneration && state.value.naming) update {
                    it.copy(faceSearchDone=true)
                }
            } catch(e: CancellationException) { throw e }
            catch(e: Exception) {
                if(repository===repo && generation==faceSearchGeneration) update { it.copy(faceMatches=emptyList(),error=message(e)) }
            } finally {
                if(repository===repo && generation==faceSearchGeneration) update { it.copy(faceSearching=false) }
            }
        }
    }
    fun assignFaceMatch(match: FaceMatch) {
        if(!editable() || !state.value.naming || match !in state.value.faceMatches) return
        task {
            search?.cancel(); cancelFaceSearch()
            // Rankings omit revisions. Fetch only the chosen target, then submit
            // through the existing revision-checked, receipted assignment flow.
            val target=repository!!.api.person(match.id)
            if(target.name!=match.name || target.name.isBlank()) throw ApiFailure(409,"conflict",UiText(R.string.error_group_changed))
            if(state.value.mergeReview) nameMerge(target=target) else mutate("assign",target=target)
        }
    }
    fun submitName(allowDuplicate: Boolean = false) { if(editable() && state.value.name.isNotBlank()) task {
        search?.cancel(); cancelFaceSearch()
        if (!allowDuplicate) {
            val duplicates = repository!!.api.suggestions(state.value.name.trim(),true)
                .filterNot { state.value.directory && !state.value.mergeReview && it.id==state.value.selectedPerson?.id }
            if (duplicates.isNotEmpty()) { update { it.copy(duplicates=duplicates) }; return@task }
        }
        if(state.value.mergeReview) nameMerge(name=state.value.name.trim(),allowDuplicate=allowDuplicate)
        else if(state.value.directory) manage("rename",name=state.value.name.trim(),allowDuplicate=allowDuplicate)
        else mutate("name",name=state.value.name.trim(),allowDuplicate=allowDuplicate)
    } }
    fun assign(target: Person) { if(editable()) task { search?.cancel(); cancelFaceSearch(); if(state.value.mergeReview) nameMerge(target=target) else mutate("assign",target=target) } }
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
                requireMessage(fresh.scope==repo.scope,R.string.error_scope_changed)
                requireMessage(fresh.namedPeople,R.string.error_people_version)
                update {it.copy(namedSearch=fresh.namedSearch)}
                requireMessage(query.isBlank() || fresh.namedSearch,R.string.error_search_version)
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
                update {it.copy(error=UiText(R.string.error_person_changed))}
            } else {
                val faces=(old.faces+page.faces).distinct()
                requireMessage(faces.size>old.faces.size,R.string.error_no_more_photos)
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
        else update {it.copy(error=UiText(R.string.error_person_changed))}
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
                        update {it.copy(error=UiText(R.string.error_ignore_unsaved))}
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
        cancelFaceSearch()
        manualMerges?.cancel()
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
            error=if(person==null) UiText(R.string.error_skipped_unavailable) else null) }
        if(person!=null) preload(person)
    } }
    fun retry() = task {
        val body=repository?.pending()?.body?.let {JSONObject(it)}
        val receipt = repository?.resolve()
        repository?.let { repo ->
            repo.restoreIgnores(repo.state().stagedIgnores.positions().map {it.id}.toSet()-ignoreJobs.keys-undoRequests)
        }
        update { it.copy(unresolved=false,naming=if(receipt!=null && (receipt.source==it.person?.id || receipt.source==it.selectedPerson?.id)) false else it.naming) }
        if(state.value.manualMerge) {
            if(receipt!=null) manualMerges?.refresh() else manualMerges?.retry()
        } else if(state.value.mergeReview) {
            if(receipt!=null && body!=null && state.value.mergePendingSide!=null) completeMergeSide(receipt,body)
            else if(state.value.mergeSideResults.isEmpty()) loadMergeSuggestion()
        }
        else if(state.value.directory) {
            if(receipt!=null && body!=null && state.value.selectedPerson!=null) applyManagementReceipt(receipt,body)
            else if(state.value.selectedPerson!=null) refreshSelectedPerson() else loadNamedPeople(true)
        } else loadNext()
    }
    fun newPass(skipped: Boolean) { if(editable()) task { repository!!.newPass(skipped); loadNext() } }
    fun openGallery() { if(!state.value.busy && photos!=null) update { it.copy(showGallery=true) } }
    fun openPeople() { if(!state.value.busy && repository!=null) update { it.copy(showGallery=false) } }
    fun switchConnection() { if(!state.value.busy) task { repository?.restoreIgnores(); session.disconnect(forget=true) } }
    private fun detachPeople() {
        manualMerges?.cancel();manualMerges=null
        ignoreJobs.values.forEach {it.cancel()};ignoreJobs.clear()
        undoRequests.clear()
        search?.cancel(); cancelFaceSearch(); directorySearch?.cancel(); searchPreload?.cancel(); statsJob?.cancel()
        preloads.forEach { it.dispose() }; preloads.clear()
        repository=null
        update { PeopleState(busy=it.busy) }
    }
    private suspend fun clearConnection() = session.disconnect()
    override fun onCleared() {
        val resources=session.detach()
        // viewModelScope is already cancelled here; resource cleanup must finish independently.
        CoroutineScope(Dispatchers.IO).launch {
            try { resources?.close() } finally { if (database.isInitialized()) database.value.close() }
        }
        super.onCleared()
    }
}
