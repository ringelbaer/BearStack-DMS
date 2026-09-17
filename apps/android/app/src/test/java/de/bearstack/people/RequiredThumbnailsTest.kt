package de.bearstack.people

import de.bearstack.people.data.remote.*
import de.bearstack.people.media.*
import kotlinx.coroutines.*
import kotlinx.coroutines.test.runTest
import org.junit.Assert.*
import org.junit.Test

class RequiredThumbnailsTest {
    private val session = PhotoSession("scope", false, 320, 240, 1280, 2048, 5, 8)
    private fun photo(id: String) = Photo(id, id, "image", "image/jpeg", "1", "2026-09-17", null, 100, 320, 240)
    private inner class Fake(val folderPages: Int = 3, val mediaCount: Int = 96) : PhotosService {
        val requests = mutableListOf<Triple<PhotoQuery, Int, String>>()
        override suspend fun session() = session
        override suspend fun browse(query: PhotoQuery, page: Int, section: String): PhotoPage {
            requests += Triple(query, page, section)
            val media = if (query.recursive) List(mediaCount) { photo("stream-$it") } else emptyList()
            val folders = if (query.recursive) emptyList() else List(24) {
                PhotoFolder("folder-$page-$it", "Folder", null, 4, false, 0,
                    List(4) { n -> photo("folder-$page-$it-$n") })
            }
            return PhotoPage("", "", page, mediaCount, query.recursive, folderPages * 24,
                !query.recursive && page < folderPages, false, media, folders, emptyList())
        }
        override suspend fun info(path: String) = photo(path)
        override suspend fun blog(path: String) = error("unused")
        override fun thumbnail(photo: Photo, size: Int) = "https://example.test/${photo.path}?size=$size"
        override fun original(photo: Photo) = error("Original must never be cached")
    }

    @Test fun exactlyLatest50AndEveryRootFolderPageArePinnedAtTheirDisplaySizes() = runTest {
        val fake = Fake()
        val required = requiredThumbnails(fake, session)
        assertEquals(50 + 3 * 24 * 4, required.size)
        assertEquals(50, required.values.count { it.url.contains("stream-") })
        assertFalse(required.values.any { it.url.contains("stream-50?") })
        assertTrue(required.values.filter { it.url.contains("stream-") }.all { it.url.endsWith("size=320") })
        assertTrue(required.values.filter { it.url.contains("folder-") }.all { it.url.endsWith("size=240") })
        assertEquals(listOf(1, 1, 2, 3), fake.requests.map { it.second })
        assertEquals(PhotoQuery(recursive = true), fake.requests.first().first)
        assertTrue(fake.requests.drop(1).all { it.first == PhotoQuery() && it.third == "folders" })
    }

    @Test fun smallStreamAndThousandsOfFoldersDoNotFetchSubfoldersOrLaterStreamPages() = runTest {
        val fake = Fake(folderPages = 100, mediaCount = 12)
        assertEquals(12 + 100 * 96, requiredThumbnails(fake, session).size)
        assertEquals(101, fake.requests.size)
    }

    @Test fun failureOrCancellationDoesNotReturnAPartialPinSet() = runTest {
        val fake = Fake()
        val broken = object : PhotosService by fake {
            override suspend fun browse(query: PhotoQuery, page: Int, section: String): PhotoPage {
                if (page == 2) throw java.io.IOException("offline")
                return fake.browse(query, page, section)
            }
        }
        try { requiredThumbnails(broken, session); fail("partial set") } catch (_: java.io.IOException) { }
        val cancelled = object : PhotosService by fake {
            override suspend fun browse(query: PhotoQuery, page: Int, section: String): PhotoPage = throw CancellationException()
        }
        try { requiredThumbnails(cancelled, session); fail("cancelled set") } catch (_: CancellationException) { }
    }
}
