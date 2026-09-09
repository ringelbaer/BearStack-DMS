package de.bearstack.people.photos

import de.bearstack.people.data.remote.*

// Keep metadata proportional to the viewport, not to the collection size.
// API page numbers remain available after eviction, so either edge can reload.
class PhotoPages<T> private constructor(
    val pageSize: Int,
    private val key: (T) -> String,
    private val pages: List<Page<T>>,
) {
    private data class Page<T>(val number: Int, val items: List<T>, val hasNext: Boolean)
    val items: List<T> = pages.flatMap { it.items }.distinctBy(key)
    val keys: List<String> = items.map(key)
    val firstPage: Int = pages.firstOrNull()?.number ?: 1
    val lastPage: Int = pages.lastOrNull()?.number ?: 1
    val hasPrevious: Boolean = firstPage > 1
    val hasNext: Boolean = pages.lastOrNull()?.hasNext == true

    fun canExtend(previous: Boolean, protectedKeys: Set<String>): Boolean = pages.size<MAX_PAGES ||
        (if(previous) pages.last() else pages.first()).items.none {key(it) in protectedKeys}

    fun add(number: Int, items: List<T>, hasNext: Boolean, reset: Boolean = false, protectedKeys: Set<String> = emptySet()): PhotoPages<T> {
        require(number > 0 && items.size <= pageSize)
        val next = Page(number, items, hasNext)
        if (reset || pages.isEmpty()) return PhotoPages(pageSize, key, listOf(next))
        require(number == firstPage - 1 || number == lastPage + 1)
        val retained = if (number < firstPage) (listOf(next) + pages).take(MAX_PAGES)
            else (pages + next).takeLast(MAX_PAGES)
        val result=PhotoPages(pageSize, key, retained)
        val retainedKeys=result.items.mapTo(HashSet(),key)
        // The viewport may have moved while the request was in flight. Discard
        // that obsolete prefetch instead of evicting something now on screen.
        if(this.items.any {key(it) in protectedKeys && key(it) !in retainedKeys}) return this
        return result
    }

    fun position(path: String): Int = pages.firstNotNullOfOrNull { page ->
        page.items.indexOfFirst { key(it) == path }.takeIf { it >= 0 }?.let { (page.number - 1) * pageSize + it + 1 }
    } ?: 0

    companion object {
        const val MAX_PAGES = 3
        fun media() = PhotoPages<Photo>(96, Photo::path, emptyList())
        fun folders() = PhotoPages<PhotoFolder>(24, PhotoFolder::path, emptyList())
        fun blogs() = PhotoPages<PhotoBlog>(20, PhotoBlog::path, emptyList())
    }
}

data class PhotoPageFailure(val previous: Boolean, val message: de.bearstack.people.text.UiText)
data class GalleryPosition(val key: String, val offset: Int = 0)
