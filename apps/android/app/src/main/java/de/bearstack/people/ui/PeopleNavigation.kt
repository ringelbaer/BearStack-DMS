package de.bearstack.people.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.relocation.BringIntoViewRequester
import androidx.compose.foundation.relocation.bringIntoViewRequester
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.foundation.rememberScrollState
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import de.bearstack.people.R
import de.bearstack.people.people.PeopleDestination
import de.bearstack.people.people.PeopleState
import de.bearstack.people.people.PeopleViewModel
import de.bearstack.people.photos.AppSettingsDialog

@Composable internal fun BackAction(onClick: () -> Unit, enabled: Boolean = true,
    description: String = stringResource(R.string.photos_back)) {
    IconButton(onClick=onClick,enabled=enabled) {Icon(painterResource(R.drawable.ic_back),description)}
}

/** Primary destinations stay visible; detail and statistics screens use Back instead. */
@Composable internal fun PeopleNavigation(selected: Int, enabled: Boolean, vm: PeopleViewModel) {
    Row(Modifier.fillMaxWidth().selectableGroup().horizontalScroll(rememberScrollState())) {
        listOf(R.string.people_tab_labeling,R.string.people_directory,R.string.people_tab_merge).forEachIndexed { index,label ->
            val active=selected==index
            val bringIntoView=remember {BringIntoViewRequester()}
            LaunchedEffect(active) {if(active) bringIntoView.bringIntoView()}
            Tab(selected=active,enabled=enabled,onClick={if(!active) vm.navigatePeople(PeopleDestination.entries[index])},
                selectedContentColor=MaterialTheme.colorScheme.onSecondaryContainer,
                unselectedContentColor=MaterialTheme.colorScheme.onSurfaceVariant,
                modifier=Modifier.widthIn(min=96.dp).bringIntoViewRequester(bringIntoView)
                    .clip(RoundedCornerShape(24.dp))
                    .background(if(active) MaterialTheme.colorScheme.secondaryContainer else Color.Transparent)
                    .testTag("people-navigation-$index"),text={Text(stringResource(label),maxLines=1)})
        }
    }
}

@Composable internal fun PeopleOptionsMenu(expanded: Boolean, onExpandedChange: (Boolean) -> Unit,
    state: PeopleState, vm: PeopleViewModel, onHelp: () -> Unit, enabled: Boolean = true,
    content: @Composable ColumnScope.() -> Unit = {}) {
    var settings by remember {mutableStateOf(false)}
    OptionsMenu(expanded,onExpandedChange,enabled=enabled) {
        content()
        DropdownMenuItem(text={Text(stringResource(R.string.photos_settings))},
            onClick={onExpandedChange(false);settings=true})
        DropdownMenuItem(text={Text(stringResource(R.string.common_help))},
            onClick={onExpandedChange(false);onHelp()})
    }
    if(settings) AppSettingsDialog(vm.photos,connected=state.connected,onConnection=vm::switchConnection,
        connectionEnabled=!state.busy,onDismiss={settings=false})
}

/** Error recovery opens the same settings and never signs out on the first tap. */
@Composable internal fun PeopleConnectionAction(state: PeopleState, vm: PeopleViewModel, enabled: Boolean = !state.busy) {
    var settings by remember {mutableStateOf(false)}
    TextButton(onClick={settings=true},enabled=enabled) {Text(stringResource(R.string.photos_settings))}
    if(settings) AppSettingsDialog(vm.photos,connected=state.connected,onConnection=vm::switchConnection,
        connectionEnabled=!state.busy,onDismiss={settings=false})
}
