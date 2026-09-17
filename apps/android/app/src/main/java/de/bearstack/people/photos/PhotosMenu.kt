package de.bearstack.people.photos

import androidx.compose.foundation.layout.Box
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import de.bearstack.people.R

/** Shared gallery actions; unavailable actions keep their place in the menu. */
@Composable
internal fun PhotosMenu(onSettings: () -> Unit, onMap: (() -> Unit)?, onFrame: (() -> Unit)?,
    onPeople: (() -> Unit)?, onDirectoryPeople: (() -> Unit)? = null,
    onOpen: () -> Unit = {}) {
    var expanded by remember { mutableStateOf(false) }
    Box {
        IconButton(onClick={ onOpen(); expanded = true }) {
            Icon(painterResource(R.drawable.ic_more_horiz), stringResource(R.string.photos_menu))
        }
        DropdownMenu(expanded, { expanded = false }) {
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
}
