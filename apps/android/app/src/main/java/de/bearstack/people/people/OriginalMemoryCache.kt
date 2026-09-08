package de.bearstack.people.people

import android.os.SystemClock
import coil.memory.MemoryCache

internal const val ORIGINAL_CACHE_PREFIX = "bearstack-original-v1:"
internal const val ORIGINAL_CACHE_TTL_MS = 180_000L

/** One bounded cache for thumbnails and originals. Hits never extend original TTLs. */
internal class OriginalMemoryCache(
    private val delegate: MemoryCache,
    private val now: () -> Long = SystemClock::elapsedRealtime,
) : MemoryCache by delegate {
    @Synchronized override fun get(key: MemoryCache.Key): MemoryCache.Value? {
        val value=delegate[key] ?: return null
        if(key.key.startsWith(ORIGINAL_CACHE_PREFIX)) {
            val inserted=value.extras[INSERTED] as? Long
            if(inserted==null || now()-inserted >= ORIGINAL_CACHE_TTL_MS) {
                delegate.remove(key)
                return null
            }
        }
        return value
    }

    @Synchronized override fun set(key: MemoryCache.Key, value: MemoryCache.Value) {
        delegate[key]=if(key.key.startsWith(ORIGINAL_CACHE_PREFIX))
            value.copy(extras=value.extras+(INSERTED to now())) else value
    }

    @Synchronized override fun remove(key: MemoryCache.Key) = delegate.remove(key)
    @Synchronized override fun clear() = delegate.clear()
    @Synchronized override fun trimMemory(level: Int) = delegate.trimMemory(level)

    private companion object { const val INSERTED = "bearstack.original.inserted" }
}
