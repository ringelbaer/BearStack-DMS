package de.bearstack.people.photos

import de.bearstack.people.text.*
import android.text.format.Formatter
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import de.bearstack.people.R
import de.bearstack.people.data.remote.Photo

@Composable internal fun DownloadPhotoAction(photo: Photo, downloads: PhotoDownloads) {
    val state by downloads.state.collectAsStateWithLifecycle()
    val picker=rememberLauncherForActivityResult(ActivityResultContracts.CreateDocument(photo.mime)) { uri ->
        val selected=downloads.pending
        downloads.pending=null
        if(uri!=null && selected!=null) downloads.save(selected,uri)
    }
    IconButton(onClick={downloads.pending=photo;picker.launch(photo.name)},enabled=!state.active) {
        Icon(painterResource(R.drawable.ic_download),stringResource(R.string.photos_download))
    }
}

@Composable internal fun PhotoDownloadStatus(downloads: PhotoDownloads) {
    val text=uiStrings()
    val state by downloads.state.collectAsStateWithLifecycle()
    val context=LocalContext.current
    if(state.active || state.complete || state.error!=null) Column(Modifier.fillMaxWidth().padding(horizontal=16.dp,vertical=8.dp)) {
        when {
            state.active -> {
                Row(Modifier.fillMaxWidth()) {
                    Text(stringResource(R.string.photos_downloading,state.name,Formatter.formatFileSize(context,state.received)),Modifier.weight(1f),style=MaterialTheme.typography.bodySmall)
                    TextButton(onClick=downloads::cancel) {Text(stringResource(R.string.photos_cancel))}
                }
                if(state.total>0) LinearProgressIndicator(progress={(state.received.toDouble()/state.total).toFloat().coerceIn(0f,1f)},modifier=Modifier.fillMaxWidth())
                else LinearProgressIndicator(Modifier.fillMaxWidth())
            }
            state.complete -> Text(stringResource(R.string.photos_downloaded,state.name),style=MaterialTheme.typography.bodySmall)
            else -> Text(state.error?.let {text(it)}.orEmpty(),color=MaterialTheme.colorScheme.error,style=MaterialTheme.typography.bodySmall)
        }
    }
}
