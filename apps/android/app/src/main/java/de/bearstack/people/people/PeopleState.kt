package de.bearstack.people.people

import de.bearstack.people.connection.CertificateOffer
import de.bearstack.people.data.local.*
import de.bearstack.people.data.remote.*
import de.bearstack.people.text.*

enum class AppSection { PEOPLE, SERVER_PHOTOS, DEVICE_PHOTOS }
enum class PeopleDestination { LABELING, DIRECTORY, MERGE }
enum class FolderOrigin { LABELING, DIRECTORY }

sealed interface PeopleRoute {
    data object Labeling : PeopleRoute
    data object Directory : PeopleRoute
    data object Merge : PeopleRoute
    data class Folders(val origin: FolderOrigin) : PeopleRoute
}

data class PersonFoldersState(
    val folderSource: Person? = null,
    val folderPage: PersonFolderPage? = null,
    val folderRequestedPage: Int = 1,
    val folderReady: Boolean = false,
    val folderSelection: PersonFolder? = null,
    val folderConfirmation: String? = null,
)

data class PeopleDirectoryState(
    val namedPeople: List<Person> = emptyList(),
    val namedDirty: Boolean = false,
    val namedCursor: Long = 0,
    val namedUpper: Long = 0,
    val namedHasNext: Boolean = false,
    val selectedPerson: Person? = null,
    val namedQuery: String = "",
    val loadedNamedQuery: String = "",
    val selectedFaces: Set<Long> = emptySet(),
    val batchNaming: Boolean = false,
    val batchConfirmation: String? = null,
    val removeFace: Long? = null,
    val removeRevision: Long = 0,
)

data class MergeReviewState(
    val mergeSuggestion: MergeSuggestion? = null,
    val mergeNamingSide: Long? = null,
    val mergePendingSide: Long? = null,
    val mergeSideResults: Map<Long,UiText> = emptyMap(),
    val mergeSidePeople: Map<Long,Person> = emptyMap(),
    val mergePendingTarget: Person? = null,
)

/** The session/writer state is shared; each workflow owns its own data. */
data class PeopleState(
    val section: AppSection = AppSection.PEOPLE,
    val route: PeopleRoute = PeopleRoute.Labeling,
    val folders: PersonFoldersState = PersonFoldersState(),
    val directoryState: PeopleDirectoryState = PeopleDirectoryState(),
    val merge: MergeReviewState = MergeReviewState(),
    val connected: Boolean = false,
    val busy: Boolean = false,
    val person: Person? = null,
    val error: UiText? = null,
    val certificate: CertificateOffer? = null,
    val naming: Boolean = false,
    val name: String = "",
    val suggestions: List<Person> = emptyList(),
    val faceMatches: List<FaceMatch> = emptyList(),
    val faceSearching: Boolean = false,
    val faceSearchDone: Boolean = false,
    val duplicates: List<Person> = emptyList(),
    val undoIgnores: List<Long> = emptyList(),
    val unresolved: Boolean = false,
    val stats: List<Statistics> = emptyList(),
    val skipped: Int = 0,
    val canGoBack: Boolean = false,
    val personFoldersSupported: Boolean = false,
    val batchFaces: Boolean = false,
    val namedSearch: Boolean = false,
    val mergeNaming: Boolean = false,
    val mergeSideActions: Boolean = false,
    val canManagePeople: Boolean = false,
    val restoring: Boolean = false,
    val savedConnection: Boolean = false,
) {
    val showGallery get() = section == AppSection.SERVER_PHOTOS
    val showDevicePhotos get() = section == AppSection.DEVICE_PHOTOS
    val directory get() = route == PeopleRoute.Directory || (route as? PeopleRoute.Folders)?.origin == FolderOrigin.DIRECTORY
    val mergeReview get() = route == PeopleRoute.Merge
    val folderReview get() = route is PeopleRoute.Folders
    val folderSource get() = folders.folderSource
    val folderPage get() = folders.folderPage
    val folderRequestedPage get() = folders.folderRequestedPage
    val folderReady get() = folders.folderReady
    val folderSelection get() = folders.folderSelection
    val folderConfirmation get() = folders.folderConfirmation
    val namedPeople get() = directoryState.namedPeople
    val namedDirty get() = directoryState.namedDirty
    val namedCursor get() = directoryState.namedCursor
    val namedUpper get() = directoryState.namedUpper
    val namedHasNext get() = directoryState.namedHasNext
    val selectedPerson get() = directoryState.selectedPerson
    val namedQuery get() = directoryState.namedQuery
    val loadedNamedQuery get() = directoryState.loadedNamedQuery
    val selectedFaces get() = directoryState.selectedFaces
    val batchNaming get() = directoryState.batchNaming
    val batchConfirmation get() = directoryState.batchConfirmation
    val removeFace get() = directoryState.removeFace
    val removeRevision get() = directoryState.removeRevision
    val mergeSuggestion get() = merge.mergeSuggestion
    val mergeNamingSide get() = merge.mergeNamingSide
    val mergePendingSide get() = merge.mergePendingSide
    val mergeSideResults get() = merge.mergeSideResults
    val mergeSidePeople get() = merge.mergeSidePeople
    val mergePendingTarget get() = merge.mergePendingTarget
}
