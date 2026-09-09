package de.bearstack.people.photos

import android.content.Context
import androidx.compose.foundation.layout.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import de.bearstack.people.R
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow

data class PlaybackSettings(val seconds: Int = 0, val frameSeconds: Int = 0, val repeat: Boolean = true,
    val frameFill: Boolean = false, val frameCaptions: Boolean = true)

class PlaybackPreferences(context: Context) {
    private val store=context.applicationContext.getSharedPreferences("photo_playback",Context.MODE_PRIVATE)
    private val mutable=MutableStateFlow(PlaybackSettings(store.getInt("seconds",0),store.getInt("frame_seconds",0),
        store.getBoolean("repeat",true),store.getBoolean("frame_fill",false),store.getBoolean("frame_captions",true)))
    val state=mutable.asStateFlow()
    fun save(settings: PlaybackSettings) {
        val value=settings.copy(seconds=if(settings.seconds==0) 0 else settings.seconds.coerceIn(3,300),
            frameSeconds=if(settings.frameSeconds==0) 0 else settings.frameSeconds.coerceIn(3,300))
        mutable.value=value
        store.edit().putInt("seconds",value.seconds).putInt("frame_seconds",value.frameSeconds).putBoolean("repeat",value.repeat)
            .putBoolean("frame_fill",value.frameFill).putBoolean("frame_captions",value.frameCaptions).apply()
    }
}

@Composable internal fun PlaybackSettingsDialog(settings: PlaybackSettings, seconds: Int, frame: Boolean,
    onSave: (PlaybackSettings) -> Unit, onDismiss: () -> Unit) {
    var draft by remember {mutableStateOf(settings)}
    var interval by remember {mutableIntStateOf(seconds)}
    var menu by remember {mutableStateOf(false)}
    AlertDialog(onDismissRequest=onDismiss,title={Text(stringResource(if(frame) R.string.photos_frame_settings else R.string.photos_slideshow_settings))},
        text={Column(verticalArrangement=Arrangement.spacedBy(12.dp)) {
            Text(stringResource(R.string.photos_interval))
            Box {
                OutlinedButton(onClick={menu=true}) {Text(stringResource(R.string.photos_seconds,interval))}
                DropdownMenu(menu,{menu=false}) {
                    listOf(3,5,8,10,15,30,60,120,300).forEach {value ->
                        DropdownMenuItem(text={Text(stringResource(R.string.photos_seconds,value))},onClick={interval=value;menu=false})
                    }
                }
            }
            PlaybackSwitch(stringResource(R.string.photos_repeat),draft.repeat) {draft=draft.copy(repeat=it)}
            if(frame) {
                PlaybackSwitch(stringResource(R.string.photos_frame_fill),draft.frameFill) {draft=draft.copy(frameFill=it)}
                PlaybackSwitch(stringResource(R.string.photos_frame_captions),draft.frameCaptions) {draft=draft.copy(frameCaptions=it)}
            }
            Text(stringResource(R.string.photos_playback_help),style=MaterialTheme.typography.bodySmall)
        }},confirmButton={TextButton(onClick={onSave(if(frame) draft.copy(frameSeconds=interval) else draft.copy(seconds=interval));onDismiss()}) {Text(stringResource(R.string.photos_save))}},
        dismissButton={TextButton(onClick=onDismiss) {Text(stringResource(R.string.photos_cancel))}})
}
@Composable private fun PlaybackSwitch(label: String, checked: Boolean, onChange: (Boolean)->Unit) {
    Row(verticalAlignment=Alignment.CenterVertically) {Text(label,Modifier.weight(1f));Switch(checked,onChange)}
}
