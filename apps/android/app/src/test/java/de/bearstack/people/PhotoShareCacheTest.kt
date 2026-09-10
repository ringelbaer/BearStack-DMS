package de.bearstack.people

import de.bearstack.people.photos.PhotoShareCache
import de.bearstack.people.text.UserIoFailure
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.File

class PhotoShareCacheTest {
    @get:Rule val temporary = TemporaryFolder()

    @Test fun largeOrUnknownLengthStreamsCannotExceedDiskLimit() {
        val cache = PhotoShareCache(temporary.root, maxFileBytes = 5, maxBytes = 10)
        val file = cache.create("jpg")
        cache.output(file).use { output ->
            output.write(byteArrayOf(1, 2, 3, 4))
            output.write(5)
            try { output.write(byteArrayOf(6, 7)); fail("oversized stream accepted") }
            catch(e: UserIoFailure) { assertEquals(R.string.photos_share_too_large, e.userText.resource) }
        }
        assertArrayEquals(byteArrayOf(1, 2, 3, 4, 5), file.readBytes())
    }

    @Test fun reservesSpaceForAnEntireOriginalAndRetainsNewestShares() {
        val cache = PhotoShareCache(temporary.root, maxFileBytes = 5, maxBytes = 12)
        val old = File(temporary.root, "old.jpg").apply { writeBytes(ByteArray(5)); setLastModified(System.currentTimeMillis() - 2000) }
        val recent = File(temporary.root, "recent.jpg").apply { writeBytes(ByteArray(5)) }
        val created = cache.create("jpg")
        cache.output(created).use { it.write(ByteArray(5)) }
        assertFalse(old.exists())
        assertTrue(recent.exists())
        assertEquals(10, temporary.root.listFiles()!!.sumOf { it.length() }.toInt())
    }

    @Test fun expiredSharesAndExcessFilesAreRemovedBeforeWriting() {
        val cache = PhotoShareCache(temporary.root, maxFiles = 2)
        val expired = File(temporary.root, "expired.jpg").apply { writeText("old"); setLastModified(1) }
        val oldest = File(temporary.root, "oldest.jpg").apply { writeText("old"); setLastModified(System.currentTimeMillis() - 2000) }
        val newest = File(temporary.root, "newest.jpg").apply { writeText("new") }
        cache.create("jpg")
        assertFalse(expired.exists())
        assertFalse(oldest.exists())
        assertTrue(newest.exists())
        assertEquals(2, temporary.root.listFiles()!!.size)
    }

    @Test fun fileNamesAreUniqueAndConfinedToTheShareDirectory() {
        val cache = PhotoShareCache(temporary.root)
        val first = cache.create("../../jpg")
        val second = cache.create("../../jpg")
        assertNotEquals(first, second)
        assertEquals(temporary.root.canonicalFile, first.canonicalFile.parentFile)
        assertTrue(first.name.endsWith(".jpg"))
    }
}
