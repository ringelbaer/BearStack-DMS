package de.bearstack.people.photos

import de.bearstack.people.R
import de.bearstack.people.data.remote.PhotoQuery

internal enum class PhotoSort(val value: String, val label: Int) {
    NEWEST("descending_date", R.string.photos_sort_newest),
    OLDEST("ascending_date", R.string.photos_sort_oldest),
    NAME_ASC("ascending_name", R.string.photos_sort_name_asc),
    NAME_DESC("descending_name", R.string.photos_sort_name_desc),
    COUNT_DESC("descending_count", R.string.photos_sort_count_desc),
    COUNT_ASC("ascending_count", R.string.photos_sort_count_asc)
}

internal fun photoSortChoices(query: PhotoQuery, tab: Int, peopleCountSort: Boolean): List<PhotoSort> {
    val parts = query.path.split('/')
    if (parts.first() == ".people" && parts.size <= 2) {
        val people = parts.size == 2 && (parts[1] == "all" || parts[1].startsWith("t-") || parts[1].startsWith("f-"))
        return listOf(PhotoSort.NAME_ASC, PhotoSort.NAME_DESC) +
            if (people && peopleCountSort) listOf(PhotoSort.COUNT_DESC, PhotoSort.COUNT_ASC) else emptyList()
    }
    if (tab == 0 || query.recursive || parts.first() == ".people") {
        return listOf(PhotoSort.NEWEST, PhotoSort.OLDEST)
    }
    return listOf(PhotoSort.NEWEST, PhotoSort.OLDEST, PhotoSort.NAME_ASC, PhotoSort.NAME_DESC)
}

internal fun normalizePhotoSort(query: PhotoQuery, tab: Int, peopleCountSort: Boolean): PhotoQuery {
    val choices = photoSortChoices(query, tab, peopleCountSort)
    return if (choices.any { it.value == query.sort }) query else query.copy(sort = choices.first().value)
}
