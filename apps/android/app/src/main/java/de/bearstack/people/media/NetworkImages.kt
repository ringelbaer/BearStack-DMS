package de.bearstack.people.media

import coil3.ImageLoader
import coil3.network.cachecontrol.CacheControlCacheStrategy
import coil3.network.okhttp.OkHttpNetworkFetcherFactory
import okhttp3.OkHttpClient

// Reuse the caller's authentication, TLS policy and pool. Map tiles supply their
// separate public client. Keep HTTP cache semantics when moving to Coil 3.
@OptIn(coil3.annotation.ExperimentalCoilApi::class)
internal fun ImageLoader.Builder.networkClient(client: OkHttpClient): ImageLoader.Builder = components {
    add(OkHttpNetworkFetcherFactory(
        callFactory = { client },
        cacheStrategy = { CacheControlCacheStrategy() },
    ))
}
