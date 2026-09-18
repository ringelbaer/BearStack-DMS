package de.bearstack.people.media

import java.io.File
import java.io.IOException
import java.security.MessageDigest
import java.nio.file.Files
import java.nio.file.StandardCopyOption
import kotlinx.coroutines.CancellationException

internal const val THUMBNAIL_MAX_BYTES = 4 * 1024 * 1024L
internal const val MIB = 1024 * 1024L

internal fun thumbnailDigest(value: String): String {
    val bytes = MessageDigest.getInstance("SHA-256").digest(value.toByteArray(Charsets.UTF_8))
    val hex = "0123456789abcdef"
    return CharArray(bytes.size * 2) { index ->
        val valueByte = bytes[index / 2].toInt() and 255
        hex[if (index % 2 == 0) valueByte ushr 4 else valueByte and 15]
    }.concatToString()
}

internal data class ThumbnailCacheUsage(val bytes: Long, val pinnedBytes: Long, val entries: Int,
    val pinnedEntries: Int, val requiredEntries: Int, val budget: Long) {
    val effectiveBudget: Long get() = maxOf(budget, pinnedBytes)
}

/** Encoded thumbnails only. Call on IO. A single connection owns this directory.
 * Pin manifests survive process death; filesystem mtimes restore LRU order.
 * The protected minimum takes precedence over the user's discretionary budget.
 */
internal class ThumbnailDiskCache(private val directory: File, budget: Long) {
    private val entries = HashMap<String, Long>()
    private val evictable = LinkedHashMap<String, Long>(16, .75f, true)
    private var pins = emptySet<String>()
    private var bytes = 0L
    private var pinnedBytes = 0L
    private var pinnedEntries = 0
    private var budget = budget.coerceAtLeast(0)
    private var closed = false

    init {
        if (!directory.isDirectory && !directory.mkdirs()) throw IOException("Cannot create thumbnail cache")
        val manifest = File(directory, "pins")
        if (manifest.isFile) pins = manifest.useLines { lines -> lines.filter(::validKey).toSet() }
        // Read filesystem metadata once per entry, never from the sort comparator.
        directory.listFiles()?.map { Triple(it, it.length(), it.lastModified()) }
            ?.sortedBy { it.third }?.forEach { (file, length, _) ->
            if (validKey(file.name) && file.isFile && length in 1..THUMBNAIL_MAX_BYTES) {
                entries[file.name] = length
                bytes += length
                if (file.name in pins) { pinnedBytes += length; pinnedEntries++ }
                else evictable[file.name] = length
            } else if (file.name != "pins") file.delete()
        }
        trim()
    }

    @Synchronized fun usage() = ThumbnailCacheUsage(bytes, pinnedBytes, entries.size,
        pinnedEntries, pins.size, budget)

    /** Validate a warm entry without reading its body or changing display recency. */
    @Synchronized fun contains(key: String): Boolean {
        checkOpen(); require(validKey(key))
        val length = entries[key] ?: return false
        val file = File(directory, key)
        if (!file.isFile || file.length() != length) { remove(key); return false }
        return true
    }

    @Synchronized fun read(key: String): ByteArray? {
        checkOpen(); require(validKey(key))
        val length = entries[key] ?: return null
        evictable[key] // Update recency without scanning the protected set.
        val file = File(directory, key)
        try {
            if (file.length() != length) { remove(key); return null }
            val data = file.readBytes()
            file.setLastModified(System.currentTimeMillis())
            return data
        } catch (_: IOException) { remove(key); return null }
    }

    @Synchronized fun write(key: String, data: ByteArray) {
        checkOpen(); require(validKey(key))
        require(data.isNotEmpty() && data.size <= THUMBNAIL_MAX_BYTES)
        if (key !in pins && data.size > (budget - pinnedBytes).coerceAtLeast(0)) return
        atomicWrite(File(directory, key), data)
        entries.remove(key)?.let { bytes -= it; if (key in pins) { pinnedBytes -= it; pinnedEntries-- } }
        entries[key] = data.size.toLong()
        bytes += data.size
        if (key in pins) { pinnedBytes += data.size; pinnedEntries++ }
        else evictable[key] = data.size.toLong()
        trim()
    }

    @Synchronized fun protect(keys: Set<String>) {
        checkOpen(); require(keys.all(::validKey))
        if (keys == pins) return
        atomicWrite(File(directory, "pins"), keys.joinToString("\n").toByteArray(Charsets.UTF_8), sync = true)
        // Newly released pins become ordinary LRU entries; existing LRU order is retained.
        (pins - keys).forEach { key -> entries[key]?.let { evictable[key] = it } }
        keys.forEach { evictable.remove(it) }
        pins = keys.toSet()
        pinnedBytes = pins.sumOf { entries[it] ?: 0L }
        pinnedEntries = pins.count { entries.containsKey(it) }
        trim()
    }

    @Synchronized fun resize(value: Long) {
        checkOpen(); budget = value.coerceAtLeast(0); trim()
    }

    @Synchronized fun close(clear: Boolean = false) {
        closed = true
        if (clear) directory.deleteRecursively()
    }

    private fun trim() {
        val iterator = evictable.iterator()
        while (bytes > maxOf(budget, pinnedBytes) && iterator.hasNext()) {
            val entry = iterator.next()
            val file = File(directory, entry.key)
            if (file.delete() || !file.exists()) { bytes -= entry.value; entries.remove(entry.key); iterator.remove() }
            else throw IOException("Cannot evict thumbnail")
        }
    }

    @Synchronized fun remove(key: String) {
        checkOpen(); require(validKey(key))
        entries.remove(key)?.let { bytes -= it; if (key in pins) { pinnedBytes -= it; pinnedEntries-- } }
        evictable.remove(key)
        File(directory, key).delete()
    }

    private fun checkOpen() { if (closed) throw CancellationException("Thumbnail cache closed") }
    private fun validKey(key: String) = key.length == 64 && key.all { it in '0'..'9' || it in 'a'..'f' }
    private fun atomicWrite(file: File, data: ByteArray, sync: Boolean = false) {
        val temp = File(directory, file.name + ".tmp")
        try {
            temp.outputStream().use { it.write(data); if (sync) it.fd.sync() }
            Files.move(temp.toPath(), file.toPath(), StandardCopyOption.ATOMIC_MOVE, StandardCopyOption.REPLACE_EXISTING)
        } finally { temp.delete() }
    }
}
