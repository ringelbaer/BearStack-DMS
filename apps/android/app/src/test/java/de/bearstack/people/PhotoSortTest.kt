package de.bearstack.people

import de.bearstack.people.data.remote.PhotoQuery
import de.bearstack.people.photos.*
import org.junit.Assert.*
import org.junit.Test

class PhotoSortTest {
    @Test fun streamAndPersonPhotosOfferOnlyDates() {
        for ((path, tab) in listOf("" to 0, ".people/all/42" to 1, ".people/t-ZmFtaWx5/42" to 1, ".people/f-/42" to 1)) {
            assertEquals(listOf(PhotoSort.NEWEST, PhotoSort.OLDEST), photoSortChoices(PhotoQuery(path=path), tab, true))
        }
        assertEquals(listOf(PhotoSort.NEWEST, PhotoSort.OLDEST), photoSortChoices(PhotoQuery(recursive=true), 2, true))
    }
    @Test fun countSortRequiresPeopleAndServerSupport() {
        for (path in listOf(".people/all", ".people/t-ZmFtaWx5", ".people/f-")) {
            assertEquals(listOf(PhotoSort.NAME_ASC, PhotoSort.NAME_DESC, PhotoSort.COUNT_DESC, PhotoSort.COUNT_ASC), photoSortChoices(PhotoQuery(path=path), 1, true))
            assertEquals(listOf(PhotoSort.NAME_ASC, PhotoSort.NAME_DESC), photoSortChoices(PhotoQuery(path=path), 1, false))
        }
        assertEquals(listOf(PhotoSort.NAME_ASC, PhotoSort.NAME_DESC), photoSortChoices(PhotoQuery(path=".people"), 1, true))
        assertEquals(listOf(PhotoSort.NEWEST, PhotoSort.OLDEST, PhotoSort.NAME_ASC, PhotoSort.NAME_DESC), photoSortChoices(PhotoQuery(path="Holiday"), 1, true))
    }
}
