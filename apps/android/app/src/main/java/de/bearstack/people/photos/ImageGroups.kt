package de.bearstack.people.photos

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.clickable
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import de.bearstack.people.R
import de.bearstack.people.data.remote.*
import de.bearstack.people.text.*
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.launch

internal fun canGroupSelection(photos: List<Photo>): Boolean {
    if(photos.size !in 2..MAX_PHOTO_SELECTION || photos.any {it.type!="image"}) return false
    return photos.map {it.imageGroupId}.filter {it>0}.distinct().size<=1 && photos.any {it.imageGroupId==0L}
}

@Composable internal fun PhotoGroupSelectionAction(controller: PhotosController, photos: List<Photo>) {
    var selected by remember(controller) {mutableStateOf<List<Photo>?>(null)}
    TextButton(enabled=canGroupSelection(photos),onClick={selected=photos.toList()}) {Text(stringResource(R.string.photos_group))}
    selected?.let { snapshot ->
        CreateImageGroupDialog(controller,snapshot,onClose={selected=null})
    }
}

@Composable private fun CreateImageGroupDialog(controller: PhotosController, photos: List<Photo>, onClose: () -> Unit) {
    var primary by remember {mutableStateOf(photos.first().path)}
    var busy by remember {mutableStateOf(false)}
    var error by remember {mutableStateOf<UiText?>(null)}
    val scope=rememberCoroutineScope()
    val existing=photos.firstOrNull {it.imageGroupId>0}?.imageGroupId
    AlertDialog(onDismissRequest={if(!busy) onClose()},title={Text(stringResource(if(existing==null) R.string.photos_group else R.string.photos_group_add))},
        text={Column {
            Text(stringResource(if(existing==null) R.string.photos_group_choose_primary else R.string.photos_group_add_description))
            if(existing==null) LazyColumn(Modifier.heightIn(max=320.dp)) {
                items(photos,key={it.path}) {photo ->
                    Row {
                        RadioButton(selected=primary==photo.path,enabled=!busy,onClick={primary=photo.path})
                        TextButton(enabled=!busy,onClick={primary=photo.path}) {Text(photo.displayPath.ifBlank {photo.name})}
                    }
                }
            }
            if(busy) LinearProgressIndicator(Modifier.fillMaxWidth())
            error?.let {Text(uiStrings()(it),color=MaterialTheme.colorScheme.error)}
        }},
        confirmButton={TextButton(enabled=!busy && error==null,onClick={
            busy=true
            scope.launch {
                try {
                    if(existing==null) controller.service.createImageGroup(photos.map {it.path},primary)
                    else {
                        val group=controller.service.imageGroup(existing)
                        controller.service.updateImageGroup(group,"add",paths=photos.filter {it.imageGroupId==0L}.map {it.path})
                    }
                    controller.open(controller.state.value.query);onClose()
                } catch(e: CancellationException) {throw e}
                catch(e: Exception) {error=failureText(e)}
                finally {busy=false}
            }
        }) {Text(stringResource(R.string.photos_group_save))}},
        dismissButton={TextButton(enabled=!busy,onClick={if(error!=null) controller.open(controller.state.value.query);onClose()}) {Text(stringResource(R.string.photos_cancel))}})
}

@Composable internal fun ImageGroupDialog(controller: PhotosController, id: Long, images: coil3.ImageLoader?, onFolder: ((Photo) -> Unit)?, onClose: () -> Unit, onChanged: () -> Unit) {
    var viewed by remember(id) {mutableStateOf<Photo?>(null)}
    if(images!=null) viewed?.let {photo -> PhotoViewer(controller,images,listOf(photo),photo.path,onClose={viewed=null},standalone=true,onOpenFolder={target ->viewed=null;onClose();onFolder?.invoke(target)})}
    var group by remember(id) {mutableStateOf<ImageGroup?>(null)}
    var error by remember(id) {mutableStateOf<UiText?>(null)}
    var busy by remember(id) {mutableStateOf(false)}
    var pending by remember(id) {mutableStateOf<Pair<String,Long>?>(null)}
    var attempt by remember(id) {mutableIntStateOf(0)}
    val scope=rememberCoroutineScope()
    LaunchedEffect(id,attempt) {
        error=null;group=null
        try {group=controller.service.imageGroup(id)}
        catch(e: CancellationException) {throw e}
        catch(e: Exception) {error=failureText(e)}
    }
    AlertDialog(onDismissRequest={if(!busy) onClose()},title={Text(stringResource(R.string.photos_group_view))},
        text={Column {
            if(group==null && error==null || busy) LinearProgressIndicator(Modifier.fillMaxWidth())
            error?.let {Text(uiStrings()(it),color=MaterialTheme.colorScheme.error);TextButton(onClick={attempt++}) {Text(stringResource(R.string.photos_retry))}}
            group?.let {value -> LazyColumn(Modifier.heightIn(max=400.dp)) {
                items(value.members,key={it.id}) {member -> Column(Modifier.padding(vertical=8.dp)) {
                    if(images!=null && !member.missing) {
                        val preview=Photo(member.path,member.displayPath,"image","image/jpeg",value.revision.toString(),"",null,0,0,0)
                        PhotoThumbnail(preview,controller,images,controller.session.thumbnailSize,Modifier.size(96.dp).clickable(enabled=!busy) {
                            scope.launch {
                                try {viewed=controller.service.info(member.path)}
                                catch(e: CancellationException) {throw e}
                                catch(e: Exception) {error=failureText(e)}
                            }
                        })
                    }
                    Text(member.displayPath)
                    if(member.primary) Text(stringResource(R.string.photos_group_primary))
                    if(member.missing) Text(stringResource(R.string.photos_group_missing))
                    if(controller.session.canManagePeople) Row {
                        if(!member.primary && !member.missing) TextButton(enabled=!busy && error==null,onClick={pending="primary" to member.id}) {Text(stringResource(R.string.photos_group_make_primary))}
                        TextButton(enabled=!busy && error==null,onClick={pending="remove" to member.id}) {Text(stringResource(R.string.photos_group_remove))}
                    }
                }}
            }}
        }},confirmButton={TextButton(enabled=!busy,onClick=onClose) {Text(stringResource(R.string.photos_close))}},
        dismissButton={if(controller.session.canManagePeople && group!=null) TextButton(enabled=!busy && error==null,onClick={pending="dissolve" to 0L}) {Text(stringResource(R.string.photos_group_dissolve))}})
    pending?.let {action -> AlertDialog(onDismissRequest={pending=null},
        title={Text(stringResource(R.string.photos_group_change))},
        text={Text(stringResource(when(action.first) {"primary" -> R.string.photos_group_make_primary; "remove" -> R.string.photos_group_remove;else -> R.string.photos_group_dissolve}))},
        confirmButton={TextButton(onClick={
            val current=group ?: return@TextButton
            pending=null;busy=true
            scope.launch {
                try {controller.service.updateImageGroup(current,action.first,action.second);onChanged()}
                catch(e: CancellationException) {throw e}
                catch(e: Exception) {error=failureText(e)}
                finally {busy=false}
            }
        }) {Text(stringResource(R.string.photos_group_save))}},
        dismissButton={TextButton(onClick={pending=null}) {Text(stringResource(R.string.photos_cancel))}})
    }
}
