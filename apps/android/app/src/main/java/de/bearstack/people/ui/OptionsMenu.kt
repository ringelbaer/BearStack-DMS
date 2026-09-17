package de.bearstack.people.ui

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.material3.*
import androidx.compose.runtime.Composable
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import de.bearstack.people.R

/** Place last in a toolbar's actions: one horizontal ellipsis and one anchor. */
@Composable internal fun OptionsMenu(expanded: Boolean, onExpandedChange: (Boolean) -> Unit,
    enabled: Boolean = true, description: String = stringResource(R.string.photos_menu),
    content: @Composable ColumnScope.() -> Unit) {
    Box {
        IconButton(onClick = { onExpandedChange(true) }, enabled = enabled) {
            Icon(painterResource(R.drawable.ic_more_horiz), description)
        }
        DropdownMenu(expanded, onDismissRequest = { onExpandedChange(false) }, content = content)
    }
}
