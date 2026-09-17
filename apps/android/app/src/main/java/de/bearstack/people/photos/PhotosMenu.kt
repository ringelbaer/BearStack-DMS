package de.bearstack.people.photos

import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.res.stringResource
import de.bearstack.people.R
import de.bearstack.people.ui.OptionsMenu

/** Shared gallery actions; unavailable actions keep their place in the menu. */
@Composable
internal fun PhotosMenu(onSettings: () -> Unit, onMap: (() -> Unit)?, onFrame: (() -> Unit)?,
    onPeople: (() -> Unit)?, onDirectoryPeople: (() -> Unit)? = null,
    onOpen: () -> Unit = {}) {
    var expanded by remember { mutableStateOf(false) }
    OptionsMenu(expanded, { if(it) onOpen(); expanded = it }) {
            if(onDirectoryPeople != null) DropdownMenuItem(
                text={ Text(stringResource(R.string.photos_directory_people)) },
                onClick={ expanded = false; onDirectoryPeople() })
            DropdownMenuItem(text={ Text(stringResource(R.string.photos_map)) }, enabled=onMap != null,
                onClick={ expanded = false; onMap?.invoke() })
            DropdownMenuItem(text={ Text(stringResource(R.string.photos_frame)) }, enabled=onFrame != null,
                onClick={ expanded = false; onFrame?.invoke() })
            if(onPeople != null) DropdownMenuItem(text={ Text(stringResource(R.string.photos_people)) },
                onClick={ expanded = false; onPeople() })
            DropdownMenuItem(text={ Text(stringResource(R.string.photos_settings)) },
                onClick={ expanded = false; onSettings() })
    }
}
