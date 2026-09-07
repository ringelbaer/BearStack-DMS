package de.bearstack.people.ui

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.*
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Popup
import androidx.compose.ui.window.PopupProperties

/** A nonmodal, clickable in-app toast; only the toast itself receives touches. */
@Composable
fun IgnoreUndoToast(count: Int, onUndo: () -> Unit) {
    Popup(alignment=Alignment.TopCenter,
        properties=PopupProperties(focusable=false,dismissOnBackPress=false,dismissOnClickOutside=false)) {
        Box(Modifier.statusBarsPadding().padding(horizontal=16.dp,vertical=8.dp)) {
            Surface(onClick=onUndo,shape=RoundedCornerShape(16.dp),
                color=MaterialTheme.colorScheme.inverseSurface,contentColor=MaterialTheme.colorScheme.inverseOnSurface,
                shadowElevation=6.dp,modifier=Modifier.widthIn(max=440.dp).testTag("ignore-undo-toast").semantics {
                    liveRegion=LiveRegionMode.Polite
                    role=Role.Button
                }) {
                Text(if(count==1) "Gruppe ignoriert · Rückgängig" else "$count Gruppen ignoriert · Letzte rückgängig",
                    modifier=Modifier.padding(horizontal=20.dp,vertical=14.dp))
            }
        }
    }
}
