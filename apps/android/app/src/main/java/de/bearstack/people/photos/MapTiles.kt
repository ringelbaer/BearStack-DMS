package de.bearstack.people.photos

import android.content.Context
import coil.ImageLoader
import coil.memory.MemoryCache
import de.bearstack.people.BuildConfig
import okhttp3.Cache
import okhttp3.Dispatcher
import okhttp3.OkHttpClient
import java.io.File
import java.io.IOException
import java.util.concurrent.TimeUnit

// Public cartography only. Never clone the authenticated BearStack client.
internal fun mapTileClient(cache: Cache): OkHttpClient = OkHttpClient.Builder()
    .cache(cache).followRedirects(false).followSslRedirects(false)
    .dispatcher(Dispatcher().apply { maxRequests=4;maxRequestsPerHost=4 })
    .connectTimeout(10,TimeUnit.SECONDS).readTimeout(15,TimeUnit.SECONDS).callTimeout(25,TimeUnit.SECONDS)
    .addInterceptor { chain ->
        val request=chain.request()
        if(request.url.scheme!="https" || request.url.host!="tile.openstreetmap.org" || request.url.port!=443)
            throw IOException("Invalid map tile host")
        chain.proceed(request.newBuilder().removeHeader("Authorization").removeHeader("Proxy-Authorization")
            .removeHeader("Cookie").header("User-Agent","BearStackPhotos/${BuildConfig.VERSION_NAME} (Android; de.bearstack.people)").build())
    }
    .addNetworkInterceptor { chain ->
        val response=chain.proceed(chain.request())
        if(response.code==200 && response.header("Cache-Control")==null && response.header("Expires")==null)
            response.newBuilder().header("Cache-Control","public, max-age=604800").build()
        else response
    }.build()

internal object MapTiles {
    @Volatile private var loader: ImageLoader? = null
    fun images(context: Context): ImageLoader = loader ?: synchronized(this) {
        loader ?: ImageLoader.Builder(context.applicationContext)
            .okHttpClient(mapTileClient(Cache(File(context.cacheDir,"map-tiles"),64L*1024*1024)))
            .diskCache(null).memoryCache {MemoryCache.Builder(context).maxSizeBytes(8*1024*1024).build()}
            .respectCacheHeaders(true).build().also {loader=it}
    }
}
