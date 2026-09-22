package de.bearstack.people.people

import de.bearstack.people.R
import de.bearstack.people.data.remote.*
import de.bearstack.people.text.*
import kotlinx.coroutines.flow.StateFlow

/** Folder review owns its workflow. The host retains the single mutation writer,
 * session lifetime and durable receipt recovery shared with other workflows. */
internal class PersonFolderController(
    private val state: StateFlow<PeopleState>,
    private val update: ((PeopleState) -> PeopleState) -> Unit,
    private val currentRepository: () -> PeopleRepository?,
    private val task: (suspend () -> Unit) -> Unit,
    private val cancelSearches: () -> Unit,
    private val refreshDirectory: suspend () -> Unit,
    private val loadNext: suspend () -> Unit,
) {
    private val repository get() = currentRepository()
    private fun editable() = state.value.connected && !state.value.busy && !state.value.unresolved
    fun openPersonFolders() {
        val source=if(state.value.directory) state.value.selectedPerson else state.value.person
        if(!editable() || state.value.naming || source==null) return
        task {
            val repo=repository ?: return@task
            val fresh=repo.api.session()
            requireMessage(fresh.scope==repo.scope,R.string.error_scope_changed)
            requireMessage(fresh.personFolders,R.string.people_folders_version)
            cancelSearches()
            update {it.copy(route=PeopleRoute.Folders(if(it.directory) FolderOrigin.DIRECTORY else FolderOrigin.LABELING),
                folders=it.folders.copy(folderSource=source,folderPage=null,folderSelection=null,folderConfirmation=null,folderReady=false,folderRequestedPage=1),
                directoryState=it.directoryState.copy(selectedFaces=emptySet()))}
            loadPersonFolders(1)
        }
    }
    suspend fun loadPersonFolders(page: Int) {
        val source=state.value.folderSource ?: return
        update {it.copy(folders=it.folders.copy(folderReady=false,folderRequestedPage=page))}
        val result=try {repository!!.api.personFolders(source.id,page)}
            catch(e: ApiFailure) {
                if(e.status!=404) throw e
                PersonFolderPage(source.id,source.name,source.revision,1,false,emptyList())
            }
        update {it.copy(folders=it.folders.copy(folderPage=result,folderReady=true,folderSelection=null,folderConfirmation=null,folderSource=source.copy(name=result.name,revision=result.revision)))}
    }
    fun personFolderPage(page: Int) {
        val current=state.value
        val old=current.folderPage ?: return
        if(!editable() || !current.folderReview || current.naming || current.folderConfirmation!=null || page<1 ||
            (page!=old.page && page!=old.page-1 && !(page==old.page+1 && old.hasNext))) return
        task {loadPersonFolders(page)}
    }
    fun requestFolderAction(folder: PersonFolder, action: String) {
        val current=state.value
        if(!editable() || !current.folderReady || !current.folderReview || current.naming || current.folderConfirmation!=null ||
            folder !in current.folderPage?.folders.orEmpty() ||
            action !in (if(folder.excluded) listOf("include") else listOf("move","unnamed","ignore","exclude"))) return
        cancelSearches()
        update {it.copy(naming=action=="move",
            name="",
            suggestions=emptyList(),
            duplicates=emptyList(),
            error=null,
            folders=it.folders.copy(folderSelection=folder,folderConfirmation=if(action=="move") null else action))}
    }
    fun cancelFolderAction() {if(editable()) update {it.copy(folders=it.folders.copy(folderSelection=null,folderConfirmation=null))}}
    fun confirmFolderAction() {
        val action=state.value.folderConfirmation ?: return
        if(editable()) task {manageFolder(action)}
    }
    suspend fun manageFolder(action: String, name: String = "", target: Person? = null) {
        val current=state.value
        val folder=current.folderSelection ?: return
        val source=current.folderSource ?: return
        val page=current.folderPage ?: return
        val repo=repository ?: return
        checkMessage(current.folderReady && folder in page.folders,R.string.error_person_changed)
        repo.prepare(source.copy(revision=page.revision),"folder_$action",name=name,target=target,directory=folder.directory)
        update {it.copy(unresolved=true)}
        repo.resolve()
        update {it.copy(unresolved=false)}
        completeFolderAction()
    }
    suspend fun completeFolderAction() {
        update {it.copy(naming=false,
            suggestions=emptyList(),
            duplicates=emptyList(),
            folders=it.folders.copy(folderSelection=null,folderConfirmation=null,folderReady=false),
            directoryState=it.directoryState.copy(namedDirty=true))}
        loadPersonFolders(1)
    }
    fun closePersonFolders() {
        if(!editable() || state.value.naming || state.value.folderConfirmation!=null) return
        task {
            update {it.copy(route=if(it.directory) PeopleRoute.Directory else PeopleRoute.Labeling,
                folders=it.folders.copy(folderSource=null,folderPage=null,folderSelection=null,folderReady=false))}
            if(state.value.directory) {
                refreshDirectory()
            } else loadNext()
        }
    }
}
