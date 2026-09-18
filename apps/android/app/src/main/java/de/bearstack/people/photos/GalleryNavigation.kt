package de.bearstack.people.photos

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.*
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.unit.dp
import de.bearstack.people.R

@Composable internal fun GalleryNavigation(tab: Int, onSelect: (Int) -> Unit) {
    Box(Modifier.fillMaxWidth().windowInsetsPadding(WindowInsets.safeDrawing.only(WindowInsetsSides.Horizontal+WindowInsetsSides.Bottom))
        .padding(horizontal=20.dp,vertical=8.dp),contentAlignment=Alignment.Center) {
        Surface(shape=RoundedCornerShape(28.dp),color=MaterialTheme.colorScheme.surfaceContainer,shadowElevation=3.dp,
            modifier=Modifier.widthIn(max=440.dp).testTag("gallery-navigation")) {
            Row(Modifier.selectableGroup().padding(4.dp),verticalAlignment=Alignment.CenterVertically) {
                listOf(R.string.photos_title to R.drawable.ic_photos,R.string.photos_folders to R.drawable.ic_folder,R.string.photos_search to R.drawable.ic_search)
                    .forEachIndexed { index,(label,icon) ->
                        val active=tab==index
                        Row(Modifier.weight(1f).clip(RoundedCornerShape(24.dp))
                            .background(if(active) MaterialTheme.colorScheme.secondaryContainer else MaterialTheme.colorScheme.surfaceContainer)
                            .selectable(selected=active,role=Role.Tab,onClick={onSelect(index)})
                            .heightIn(min=48.dp).padding(horizontal=4.dp,vertical=4.dp),
                            verticalAlignment=Alignment.CenterVertically,horizontalArrangement=Arrangement.Center) {
                            Icon(painterResource(icon),null,Modifier.size(18.dp).testTag("gallery-navigation-icon-$index"),
                                tint=if(active) MaterialTheme.colorScheme.onSecondaryContainer else MaterialTheme.colorScheme.onSurfaceVariant)
                            Spacer(Modifier.width(4.dp))
                            Text(stringResource(label),style=MaterialTheme.typography.labelMedium,
                                color=if(active) MaterialTheme.colorScheme.onSecondaryContainer else MaterialTheme.colorScheme.onSurfaceVariant)
                        }
                    }
            }
        }
    }
}
