package de.bearstack.people.photos

import android.text.format.Formatter
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalResources
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import de.bearstack.people.R
import de.bearstack.people.data.remote.*
import de.bearstack.people.text.*
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.ensureActive
import java.text.NumberFormat
import java.time.OffsetDateTime
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle
import java.util.Locale
import kotlin.coroutines.coroutineContext

// Preserve the source offset: a travel photo's capture time must not change
// with the phone's current time zone. Date grouping uses that same source date.
internal fun photoTimeLabel(date: String, locale: Locale): String? = runCatching {
    val value=OffsetDateTime.parse(date)
    val time=value.format(DateTimeFormatter.ofLocalizedTime(FormatStyle.MEDIUM).withLocale(locale))
    "$time · UTC${value.offset.id.takeUnless {it=="Z"}.orEmpty()}"
}.getOrNull()

@OptIn(ExperimentalMaterial3Api::class)
@Composable internal fun PhotoInfoSheet(photo: Photo, service: PhotosService, onClose: () -> Unit) {
    val text=uiStrings()
    val context=LocalContext.current
    val configuration=LocalResources.current.configuration
    val locale=configuration.locales[0]
    val formatContext=remember(context,configuration) {context.createConfigurationContext(configuration)}
    var detail by remember(service,photo.path,photo.version) {mutableStateOf<Photo?>(null)}
    var error by remember(service,photo.path,photo.version) {mutableStateOf<UiText?>(null)}
    var attempt by remember(service,photo.path,photo.version) {mutableIntStateOf(0)}
    LaunchedEffect(service,photo.path,photo.version,attempt) {
        error=null
        try {
            val result=service.info(photo.path)
            coroutineContext.ensureActive()
            detail=result
        } catch(e: CancellationException) {throw e}
        catch(e: Exception) {error=failureText(e)}
    }
    val item=detail ?: photo
    ModalBottomSheet(onDismissRequest=onClose,sheetState=rememberModalBottomSheetState(skipPartiallyExpanded=true)) {
        Column(Modifier.fillMaxWidth().verticalScroll(rememberScrollState()).padding(horizontal=24.dp).padding(bottom=32.dp),
            verticalArrangement=Arrangement.spacedBy(20.dp)) {
            Column(verticalArrangement=Arrangement.spacedBy(6.dp)) {
                Text(photoDateLabel(item.date,locale),style=MaterialTheme.typography.headlineSmall)
                photoTimeLabel(item.date,locale)?.let {Text(it,style=MaterialTheme.typography.titleMedium)}
                Text(stringResource(if(item.captured==null) R.string.photos_modified_time else R.string.photos_capture_time),
                    style=MaterialTheme.typography.bodySmall,color=MaterialTheme.colorScheme.onSurfaceVariant)
            }
            if(detail==null && error==null) LinearProgressIndicator(Modifier.fillMaxWidth())
            error?.let {
                Column {
                    Text(text(it),color=MaterialTheme.colorScheme.error)
                    TextButton(onClick={attempt++}) {Text(stringResource(R.string.photos_retry))}
                }
            }
            InfoSection(stringResource(R.string.photos_file)) {
                Text(item.name,style=MaterialTheme.typography.titleMedium)
                val bytes=Formatter.formatFileSize(formatContext,item.bytes)
                Text(if(item.width>0 && item.height>0) stringResource(R.string.photos_details,item.width,item.height,bytes) else bytes)
                Text(item.path,style=MaterialTheme.typography.bodySmall)
            }
            if(item.camera.isNotBlank() || item.lens.isNotBlank()) InfoSection(stringResource(R.string.photos_camera)) {
                Text(listOf(item.camera,item.lens).filter(String::isNotBlank).joinToString("\n"))
            }
            item.rating?.let {rating ->
                InfoSection(stringResource(R.string.photos_rating)) {
                    Text(when {
                        rating<0 -> stringResource(R.string.photos_rating_rejected)
                        rating==0.0 -> stringResource(R.string.photos_rating_none)
                        else -> stringResource(R.string.photos_rating_stars,NumberFormat.getNumberInstance(locale).apply {maximumFractionDigits=1}.format(rating))
                    })
                }
            }
            if(item.people.isNotEmpty()) InfoSection(stringResource(R.string.photos_info_people)) {
                val unnamed=stringResource(R.string.photos_person_unnamed)
                Text(item.people.joinToString(" · ") {it.ifBlank {unnamed}})
            }
            if(item.tags.isNotEmpty()) InfoSection(stringResource(R.string.photos_tags)) {Text(item.tags.joinToString(" · "))}
            if(item.keywords.isNotEmpty()) InfoSection(stringResource(R.string.photos_keywords)) {Text(item.keywords.joinToString(" · "))}
            if(item.latitude!=null && item.longitude!=null) InfoSection(stringResource(R.string.photos_location)) {
                Text(String.format(locale,"%.5f, %.5f",item.latitude,item.longitude))
                key(item.path) {
                    PhotoMap(PhotoMapBounds(item.latitude,item.longitude,item.latitude,item.longitude),
                        listOf(PhotoMapMarker(item.latitude,item.longitude,1)),Modifier.fillMaxWidth().height(240.dp))
                }
            }
        }
    }
}

@Composable private fun InfoSection(title: String, content: @Composable ColumnScope.() -> Unit) {
    Column(verticalArrangement=Arrangement.spacedBy(6.dp)) {
        Text(title,style=MaterialTheme.typography.labelMedium,color=MaterialTheme.colorScheme.primary)
        content()
    }
}
