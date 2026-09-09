package de.bearstack.people.photos

import android.content.Context
import coil.request.CachePolicy
import coil.request.ImageRequest
import coil.size.Scale
import de.bearstack.people.data.remote.Photo
import de.bearstack.people.data.remote.PhotoSession
import de.bearstack.people.data.remote.PhotosService
import de.bearstack.people.media.ORIGINAL_CACHE_PREFIX

internal fun photoPreviewRequest(context: Context, photo: Photo, service: PhotosService, session: PhotoSession): ImageRequest {
    val url=service.thumbnail(photo,session.largePreviewSize)
    return ImageRequest.Builder(context).data(url).size(2048).scale(Scale.FIT)
        .memoryCacheKey(ORIGINAL_CACHE_PREFIX+session.scope+":"+url).diskCachePolicy(CachePolicy.DISABLED).build()
}
