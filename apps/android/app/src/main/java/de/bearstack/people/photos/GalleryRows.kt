package de.bearstack.people.photos

import de.bearstack.people.data.remote.*

internal sealed interface GalleryRow {
    val key: String
    data class Folder(val value: PhotoFolder) : GalleryRow { override val key="folder:${value.path}" }
    data class Blog(val value: PhotoBlog) : GalleryRow { override val key="blog:${value.path}" }
    data class Media(val value: Photo) : GalleryRow { override val key="photo:${value.path}" }
    data class Date(val value: Photo) : GalleryRow { override val key="date:${value.path}" }
    data object Texts : GalleryRow { override val key="texts" }
    data class Boundary(val section: String,val previous: Boolean) : GalleryRow {
        override val key="${if(previous) "previous" else "more"}-$section"
    }
}

internal fun galleryRows(state: PhotosState): List<GalleryRow> = buildList {
    fun boundary(section: String,previous: Boolean) {
        val pages=state.section(section)
        if(if(previous) pages.hasPrevious else pages.hasNext) add(GalleryRow.Boundary(section,previous))
    }
    boundary("folders",true)
    state.folders.forEach {add(GalleryRow.Folder(it))}
    boundary("folders",false)
    if(state.blogs.isNotEmpty()) add(GalleryRow.Texts)
    boundary("blogs",true)
    state.blogs.forEach {add(GalleryRow.Blog(it))}
    boundary("blogs",false)
    boundary("media",true)
    state.media.forEachIndexed {index,photo ->
        if(index==0 || state.media[index-1].date.take(10)!=photo.date.take(10)) add(GalleryRow.Date(photo))
        add(GalleryRow.Media(photo))
    }
    boundary("media",false)
}
