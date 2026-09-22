package de.bearstack.people.photos

import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.res.stringResource
import de.bearstack.people.R
import de.bearstack.people.ui.OptionsMenu

/** Null actions are inapplicable; loading disables applicable actions in place. */
@Composable internal fun PhotosMenu(onSettings: () -> Unit, onMap: (() -> Unit)?, onFrame: (() -> Unit)?,
    onPeople: (() -> Unit)?, onDirectoryPeople: (() -> Unit)? = null,
    actionsEnabled: Boolean = true, frameEnabled: Boolean = true, onOpen: () -> Unit = {}) {
    var expanded by remember {mutableStateOf(false)}
    var help by remember {mutableStateOf(false)}
    OptionsMenu(expanded,{if(it) onOpen();expanded=it}) {
        if(onDirectoryPeople!=null) DropdownMenuItem(text={Text(stringResource(R.string.photos_directory_people))},
            enabled=actionsEnabled,onClick={expanded=false;onDirectoryPeople()})
        if(onMap!=null) DropdownMenuItem(text={Text(stringResource(R.string.photos_map))},
            enabled=actionsEnabled,onClick={expanded=false;onMap()})
        if(onFrame!=null) DropdownMenuItem(text={Text(stringResource(R.string.photos_frame))},
            enabled=actionsEnabled && frameEnabled,onClick={expanded=false;onFrame()})
        if(onDirectoryPeople!=null || onMap!=null || onFrame!=null) HorizontalDivider()
        if(onPeople!=null) {
            DropdownMenuItem(text={Text(stringResource(R.string.photos_people))},onClick={expanded=false;onPeople()})
            HorizontalDivider()
        }
        DropdownMenuItem(text={Text(stringResource(R.string.photos_settings))},onClick={expanded=false;onSettings()})
        DropdownMenuItem(text={Text(stringResource(R.string.common_help))},onClick={expanded=false;help=true})
    }
    if(help) AlertDialog(onDismissRequest={help=false},title={Text(stringResource(R.string.common_help))},
        text={Text(stringResource(R.string.photos_menu_help))},
        confirmButton={TextButton(onClick={help=false}) {Text(stringResource(R.string.photos_close))}})
}
