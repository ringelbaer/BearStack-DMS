package de.bearstack.people.photos

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.grid.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import coil.ImageLoader
import de.bearstack.people.R
import de.bearstack.people.data.remote.*
import de.bearstack.people.text.*

@Composable
internal fun PhotoGallery(controller: PhotosController, images: ImageLoader, state: PhotosState, modifier: Modifier = Modifier.fillMaxSize()) {
    val configuration=LocalConfiguration.current
    val locale=configuration.locales[0]
    val columns=if(configuration.screenWidthDp>=600) 12 else 6
    val folderSpan=if(columns==6 && configuration.fontScale>=1.5f) 6 else 3
    key(state.query,state.frame) {
        val rows=remember(state.mediaPages,state.folderPages,state.blogPages,state.pageErrors) {galleryRows(state)}
        val latestCatalog by rememberUpdatedState(state)
        var pageAnchor by remember {mutableStateOf<GalleryPageAnchor?>(null)}
        val saved=remember {controller.gridPosition}
        val grid = rememberLazyGridState(
            initialFirstVisibleItemIndex=rows.indexOfFirst {it.key==saved?.key}.coerceAtLeast(0),
            initialFirstVisibleItemScrollOffset=saved?.offset ?: 0)
        fun load(section: String,previous: Boolean) {
            val current=controller.state.value
            if(section in current.loadingSections) return
            val anchor=grid.layoutInfo.visibleItemsInfo.firstOrNull {
                val key=it.key.toString()
                key.startsWith("photo:") || key.startsWith("folder:") || key.startsWith("blog:")
            }
            if(anchor!=null) pageAnchor=GalleryPageAnchor(section,current.section(section),GalleryPosition(anchor.key.toString(),-anchor.offset.y))
            if(previous) controller.previous(section) else controller.more(section)
        }
        LaunchedEffect(rows,state.loadingSections,pageAnchor) {
            val anchor=pageAnchor ?: return@LaunchedEffect
            if(state.section(anchor.section)!==anchor.pages) {
                val index=rows.indexOfFirst {it.key==anchor.position.key}
                if(index>=0 && !grid.isScrollInProgress) grid.scrollToItem(index,anchor.position.offset)
                pageAnchor=null
            } else if(anchor.section !in state.loadingSections) pageAnchor=null
        }
        LaunchedEffect(grid,state.selected,state.frame) {
            if(state.selected==null && !state.frame) snapshotFlow {
                val visible=grid.layoutInfo.visibleItemsInfo
                val content=visible.firstOrNull {
                    val key=it.key.toString()
                    key.startsWith("photo:") || key.startsWith("folder:") || key.startsWith("blog:")
                }
                GalleryViewport(
                    visible.firstOrNull()?.let {GalleryPosition(it.key.toString(),grid.firstVisibleItemScrollOffset)},
                    visible.mapTo(HashSet()) {it.key.toString()},
                    content?.let {GalleryPosition(it.key.toString(),-it.offset.y)},grid.isScrollInProgress)
            }.collect {viewport ->
                viewport.position?.let {controller.gridPosition=it}
                controller.galleryVisible(viewport.keys)
                pageAnchor?.let {anchor ->
                    if(viewport.scrolling || (viewport.content!=null && viewport.content.key!=anchor.position.key)) anchor.userScrolled=true
                    if(anchor.userScrolled && latestCatalog.section(anchor.section)===anchor.pages) {
                        viewport.content?.let {anchor.position=it}
                    }
                }
            }
        }
        // Start loading while several rows are still ahead of the viewport. Network
        // batches have no visible boundary and never require a navigation button.
        LaunchedEffect(grid) {
            snapshotFlow {
                if(pageAnchor!=null) null else galleryPrefetch(latestCatalog,
                    grid.layoutInfo.visibleItemsInfo.mapTo(HashSet()) {it.key.toString()})
            }.collect {request -> request?.let {load(it.section,it.previous)} }
        }
        LaunchedEffect(state.scrollToKey,rows) {
            val target=state.scrollToKey ?: return@LaunchedEffect
            val index=rows.indexOfFirst {it.key==target}
            if(index>=0) {grid.scrollToItem(index);controller.scrollConsumed()}
        }
        LazyVerticalGrid(columns=GridCells.Fixed(columns),state=grid,modifier=modifier.testTag("photo-gallery"),
            horizontalArrangement=Arrangement.spacedBy(2.dp),verticalArrangement=Arrangement.spacedBy(2.dp),contentPadding=PaddingValues(bottom=16.dp)) {
            items(rows,key={it.key},span={GridItemSpan(when(it) {
                is GalleryRow.Folder -> folderSpan; is GalleryRow.Media -> 2; else -> maxLineSpan
            })},contentType={when(it) {
                is GalleryRow.Folder -> "folder"; is GalleryRow.Media -> "photo"; is GalleryRow.Blog -> "blog"
                is GalleryRow.Failure -> "failure"; else -> "heading"
            }}) {row ->
                when(row) {
                    is GalleryRow.Folder -> FolderTile(row.value,controller,images) {
                        controller.open(PhotoQuery(path=row.value.path),tab=1)
                    }
                    is GalleryRow.Blog -> ListItem(headlineContent={Text(row.value.name)},
                        supportingContent={Text(photoDateLabel(row.value.date ?: row.value.modified,locale))},
                        modifier=Modifier.clickable {controller.openBlog(row.value)})
                    is GalleryRow.Media -> PhotoThumbnail(row.value,controller,images,controller.session.thumbnailSize,
                        Modifier.fillMaxWidth().aspectRatio(1f).clickable {controller.select(row.value.path)})
                    is GalleryRow.Date -> SectionTitle(photoDateLabel(row.value.date,locale))
                    GalleryRow.Texts -> SectionTitle(stringResource(R.string.photos_texts))
                    is GalleryRow.Failure -> GalleryLoadError(state.pageErrors.getValue(row.section).message) {
                        load(row.section,row.previous)
                    }
                }
            }
            if(!state.loading && state.media.isEmpty() && state.folders.isEmpty() && state.blogs.isEmpty() && state.error==null) {
                item(span={GridItemSpan(maxLineSpan)}) {
                    Column(Modifier.fillMaxWidth().padding(40.dp),horizontalAlignment=Alignment.CenterHorizontally,verticalArrangement=Arrangement.spacedBy(16.dp)) {
                        Icon(painterResource(R.drawable.ic_photos),null,Modifier.size(48.dp),tint=MaterialTheme.colorScheme.outline)
                        Text(stringResource(if(state.query.query.isBlank()) R.string.photos_empty else R.string.photos_no_results))
                    }
                }
            }
        }
    }
}

@Composable
private fun GalleryLoadError(message: UiText,onRetry: ()->Unit) {
    val text=uiStrings()
    Row(Modifier.fillMaxWidth().padding(12.dp),verticalAlignment=Alignment.CenterVertically) {
        Text(text(message),Modifier.weight(1f),color=MaterialTheme.colorScheme.error)
        TextButton(onClick=onRetry) {Text(stringResource(R.string.photos_retry))}
    }
}

private class GalleryPageAnchor(val section: String,val pages: PhotoPages<*>,var position: GalleryPosition) {
    var userScrolled=false
}
private data class GalleryViewport(val position: GalleryPosition?,val keys: Set<String>,val content: GalleryPosition?,val scrolling: Boolean)
