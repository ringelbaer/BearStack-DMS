package de.bearstack.people.media

import android.content.Context
import coil3.ImageLoader
import coil3.memory.MemoryCache
import coil3.request.CachePolicy
import okhttp3.OkHttpClient

internal fun sessionImages(context: Context, client: OkHttpClient, thumbnails: ThumbnailCache?): ImageLoader =
    ImageLoader.Builder(context)
        .networkClient(client) {
            thumbnails?.let {
                add(ThumbnailCache.Factory(it))
                add(ThumbnailCache.Keys())
                add(ThumbnailCache.Integrity(it))
            }
        }
        .diskCachePolicy(CachePolicy.DISABLED)
        .memoryCache {
            OriginalMemoryCache(MemoryCache.Builder().maxSizeBytes(16 * 1024 * 1024)
                .weakReferencesEnabled(false).build())
        }.build()
