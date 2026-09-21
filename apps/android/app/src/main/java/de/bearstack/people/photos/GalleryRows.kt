package de.bearstack.people.photos

import de.bearstack.people.data.remote.*

internal sealed interface GalleryRow {
    val key: String
    data class Folder(val value: PhotoFolder) : GalleryRow { override val key="folder:${value.path}" }
    data class Blog(val value: PhotoBlog) : GalleryRow { override val key="blog:${value.path}" }
    data class Media(val value: Photo) : GalleryRow { override val key="photo:${value.path}" }
    data class Date(val value: Photo) : GalleryRow { override val key="date:${value.path}" }
    data object Texts : GalleryRow { override val key="texts" }
    data object Device : GalleryRow { override val key="local-device" }
    data class Failure(val section: String,val previous: Boolean) : GalleryRow {
        override val key="error-$section"
    }
}

internal fun galleryRows(state: PhotosState): List<GalleryRow> = buildList {
    fun boundary(section: String,previous: Boolean) {
        if(state.pageErrors[section]?.previous==previous) add(GalleryRow.Failure(section,previous))
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

internal data class GalleryPrefetch(val section: String,val previous: Boolean)

internal fun galleryPrefetch(state: PhotosState,visibleKeys: Set<String>): GalleryPrefetch? {
    if(state.loading || state.dateLoading || state.seekLoading || state.selected!=null || state.frame || state.scrollToKey!=null) return null
    for((section,prefix) in listOf("folders" to "folder:","blogs" to "blog:","media" to "photo:")) {
        if(section in state.loadingSections) continue
        val window=state.section(section)
        val keys=window.keys
        val indices=keys.indices.filter {prefix+keys[it] in visibleKeys}
        if(indices.isEmpty()) continue
        val ahead=when(section) {"media" -> 18; "folders" -> 6; else -> 6}
        val protected=indices.mapTo(HashSet()) {keys[it]}
        if(window.hasPrevious && indices.first()<ahead && state.pageErrors[section]?.previous!=true && window.canExtend(true,protected))
            return GalleryPrefetch(section,true)
        if(window.hasNext && indices.last()>=keys.size-ahead && state.pageErrors[section]?.previous!=false && window.canExtend(false,protected))
            return GalleryPrefetch(section,false)
    }
    return null
}

internal data class GallerySeekTarget(val section: String,val index: Int)
internal fun galleryItemCount(state: PhotosState) = (state.folderTotal.toLong()+state.total).coerceIn(0,Int.MAX_VALUE.toLong()).toInt()
internal fun gallerySeekTarget(state: PhotosState,position: Int): GallerySeekTarget? {
    val count=galleryItemCount(state)
    if(count==0) return null
    val index=position.coerceIn(0,count-1)
    return if(index<state.folderTotal) GallerySeekTarget("folders",index) else GallerySeekTarget("media",index-state.folderTotal)
}
internal fun galleryItemPosition(state: PhotosState,key: String): Int? = when {
    key.startsWith("folder:") -> state.folderPages.position(key.removePrefix("folder:")).takeIf {it>0}?.minus(1)
    key.startsWith("photo:") -> state.mediaPages.position(key.removePrefix("photo:")).takeIf {it>0}?.let {state.folderTotal+it-1}
    else -> null
}
