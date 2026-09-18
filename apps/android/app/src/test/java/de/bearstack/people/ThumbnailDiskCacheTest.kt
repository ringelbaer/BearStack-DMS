package de.bearstack.people

import de.bearstack.people.media.*
import java.io.File
import kotlinx.coroutines.CancellationException
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder

class ThumbnailDiskCacheTest {
    @get:Rule val temp = TemporaryFolder()
    private fun key(value: String) = thumbnailDigest(value)

    @Test fun lruEvictsOnlyUnprotectedEntriesAndReadsPromote() {
        val cache = ThumbnailDiskCache(temp.root, 12)
        cache.protect(setOf(key("pin")))
        for (id in listOf("pin", "a", "b")) cache.write(key(id), ByteArray(4))
        cache.read(key("a"))
        cache.write(key("c"), ByteArray(4))
        assertNull(cache.read(key("b")))
        assertNotNull(cache.read(key("a"))); assertNotNull(cache.read(key("pin")))
        assertEquals(12L, cache.usage().bytes)
        assertEquals(4L, cache.usage().pinnedBytes)
    }

    @Test fun protectedMinimumSurvivesResizeAndRestartThenReleasesOldPins() {
        var cache = ThumbnailDiskCache(temp.root, 20)
        cache.protect(setOf(key("one"), key("two")))
        cache.write(key("one"), ByteArray(6)); cache.write(key("two"), ByteArray(6))
        cache.write(key("other"), ByteArray(6)); cache.resize(4)
        assertNull(cache.read(key("other")))
        cache.close()
        cache = ThumbnailDiskCache(temp.root, 4)
        assertEquals(12L, cache.usage().effectiveBudget)
        assertNotNull(cache.read(key("one"))); assertNotNull(cache.read(key("two")))
        cache.protect(setOf(key("two")))
        assertNull(cache.read(key("one"))); assertEquals(6L, cache.usage().bytes)
        cache.protect(emptySet())
        assertEquals(0L, cache.usage().bytes)
    }

    @Test fun restartRestoresLruOrderingAndIgnoresIncompleteWrites() {
        var cache = ThumbnailDiskCache(temp.root, 8)
        cache.write(key("old"), ByteArray(4)); cache.write(key("new"), ByteArray(4))
        File(temp.root, key("old")).setLastModified(1000)
        File(temp.root, key("new")).setLastModified(2000)
        File(temp.root, "pins.tmp").writeText(key("old"))
        cache.close(); cache = ThumbnailDiskCache(temp.root, 8)
        cache.write(key("next"), ByteArray(4))
        assertNull(cache.read(key("old"))); assertNotNull(cache.read(key("new")))
        assertFalse(File(temp.root, "pins.tmp").exists())
    }

    @Test fun replacementCountsBytesOnceAndMissingOrTruncatedEntriesAreMisses() {
        val cache = ThumbnailDiskCache(temp.root, 32)
        cache.protect(setOf(key("a"), key("missing")))
        cache.write(key("a"), ByteArray(6)); cache.write(key("a"), ByteArray(10))
        assertEquals(10L, cache.usage().pinnedBytes)
        assertEquals(1, cache.usage().pinnedEntries); assertEquals(2, cache.usage().requiredEntries)
        File(temp.root, key("a")).writeBytes(ByteArray(2))
        assertNull(cache.read(key("a"))); assertEquals(0L, cache.usage().bytes)
        assertEquals(0, cache.usage().pinnedEntries)
    }

    @Test fun warmChecksPreserveFilesAndRecencyButRejectMissingAndTruncatedEntries() {
        val cache = ThumbnailDiskCache(temp.root, 32)
        val keys = setOf(key("a"), key("b"))
        cache.protect(keys)
        keys.forEach { cache.write(it, ByteArray(8)) }
        val manifest = File(temp.root, "pins")
        manifest.setLastModified(1000)
        keys.forEach { File(temp.root, it).setLastModified(1000) }
        repeat(3) {
            cache.protect(keys)
            keys.forEach { assertTrue(cache.contains(it)) }
        }
        assertEquals(1000L, manifest.lastModified())
        keys.forEach { assertEquals(1000L, File(temp.root, it).lastModified()) }
        File(temp.root, key("a")).delete()
        File(temp.root, key("b")).writeBytes(ByteArray(2))
        keys.forEach { assertFalse(cache.contains(it)) }
        assertEquals(0L, cache.usage().bytes)
        assertEquals(0, cache.usage().pinnedEntries)
        cache.write(key("a"), ByteArray(8))
        assertTrue(cache.contains(key("a")))
    }

    @Test fun oversizedDiscretionaryEntryDoesNotFlushUsefulEntries() {
        val cache = ThumbnailDiskCache(temp.root, 8)
        cache.write(key("small"), ByteArray(4)); cache.write(key("large"), ByteArray(9))
        assertNotNull(cache.read(key("small"))); assertNull(cache.read(key("large")))
    }

    @Test fun accountDirectoriesAreIsolatedAndExplicitCloseClears() {
        val first = ThumbnailDiskCache(File(temp.root, "first"), 32)
        val second = ThumbnailDiskCache(File(temp.root, "second"), 32)
        first.write(key("same-url"), ByteArray(8))
        assertNull(second.read(key("same-url")))
        first.close(clear = true)
        assertFalse(File(temp.root, "first").exists()); assertTrue(File(temp.root, "second").isDirectory)
        try { first.write(key("late"), ByteArray(8)); fail("closed cache accepted a late write") }
        catch (_: CancellationException) { }
    }

    @Test fun fileNamesCannotEscapeCacheDirectory() {
        val cache = ThumbnailDiskCache(temp.root, 32)
        try { cache.write("../escape", ByteArray(8)); fail("unsafe key") }
        catch (_: IllegalArgumentException) { }
    }

    @Test fun failedWritesDoNotPublishPartialDataOrReplaceThePinManifest() {
        val cache = ThumbnailDiskCache(temp.root, 32)
        cache.protect(setOf(key("old")))
        cache.write(key("old"), ByteArray(8))
        // An unwritable target simulates storage failure without relying on OS permissions.
        File(temp.root, key("new") + ".tmp").mkdir()
        try { cache.write(key("new"), ByteArray(8)); fail("write should fail") }
        catch (_: java.io.IOException) { }
        assertNull(cache.read(key("new"))); assertEquals(8L, cache.usage().bytes)
        File(temp.root, "pins.tmp").mkdir()
        try { cache.protect(setOf(key("new"))); fail("manifest should fail") }
        catch (_: java.io.IOException) { }
        assertEquals(1, cache.usage().pinnedEntries)
        cache.close()
        assertEquals(1, ThumbnailDiskCache(temp.root, 1).usage().pinnedEntries)
    }

    @Test fun versionAccountAndRequestedSizeArePartOfIdentity() {
        val base = CachedThumbnail("https://example.test/thumb?size=240", "account:1")
        assertNotEquals(base.key, base.copy(revision = "account:2").key)
        assertNotEquals(base.key, base.copy(revision = "other-account:1").key)
        assertNotEquals(base.key, base.copy(url = "https://example.test/thumb?size=320").key)
    }
}
