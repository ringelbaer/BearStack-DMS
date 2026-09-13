package de.bearstack.people.ui

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.*
import androidx.compose.runtime.Composable
import androidx.compose.runtime.key
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.semantics.*
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import coil.ImageLoader
import coil.compose.AsyncImage
import de.bearstack.people.R
import de.bearstack.people.data.remote.FaceMatch
import de.bearstack.people.people.MergeFaceMatches
import de.bearstack.people.text.uiStrings

@Composable
internal fun MergeFaceMatchList(result: MergeFaceMatches, enabled: Boolean,
    images: ImageLoader?, image: (Long, Boolean) -> String?, onAssign: (FaceMatch) -> Unit, onRetry: () -> Unit) {
    val text=uiStrings()
    Column(Modifier.fillMaxWidth().testTag("merge-matches"),verticalArrangement=Arrangement.spacedBy(12.dp)) {
        Row(verticalAlignment=Alignment.CenterVertically,horizontalArrangement=Arrangement.spacedBy(6.dp)) {
            Icon(painterResource(R.drawable.ic_search),null,Modifier.size(18.dp),tint=MaterialTheme.colorScheme.primary)
            Text(text(R.string.people_merge_matches_title),style=MaterialTheme.typography.labelLarge)
        }
        if(result.loading) {
            LinearProgressIndicator(Modifier.fillMaxWidth())
            Text(if(result.matches.isEmpty()) text(R.string.people_merge_matches_loading)
                else text(R.string.people_face_search_progress,result.matches.size),
                style=MaterialTheme.typography.bodySmall,modifier=Modifier.semantics {liveRegion=LiveRegionMode.Polite})
        } else if(result.complete && result.matches.isEmpty()) {
            Text(text(R.string.people_face_search_empty),style=MaterialTheme.typography.bodySmall)
        }
        result.error?.let {
            Text(text(it),color=MaterialTheme.colorScheme.error,style=MaterialTheme.typography.bodySmall,
                modifier=Modifier.semantics {liveRegion=LiveRegionMode.Polite})
        }
        result.matches.forEach { match -> key(match.id) {
            Surface(onClick={onAssign(match)},enabled=enabled,shape=RoundedCornerShape(12.dp),
                color=MaterialTheme.colorScheme.surfaceContainer,
                modifier=Modifier.fillMaxWidth().testTag("merge-match-${match.id}")
                    .semantics {contentDescription=text(R.string.people_merge_match_assign,match.name)}) {
                Row(Modifier.heightIn(min=72.dp).padding(12.dp),verticalAlignment=Alignment.CenterVertically,
                    horizontalArrangement=Arrangement.spacedBy(12.dp)) {
                    if(images!=null) AsyncImage(image(match.faceId,false),null,imageLoader=images,
                        contentScale=ContentScale.Crop,modifier=Modifier.size(48.dp).clip(RoundedCornerShape(8.dp)))
                    Column(Modifier.weight(1f),verticalArrangement=Arrangement.spacedBy(4.dp)) {
                        Text(match.name,style=MaterialTheme.typography.bodyMedium,maxLines=2,overflow=TextOverflow.Ellipsis)
                        Text(text(R.string.people_face_count_id,text.faces(match.count),match.id),
                            style=MaterialTheme.typography.labelSmall,color=MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                }
            }
        } }
        if(!result.loading) TextButton(onClick=onRetry,enabled=enabled,modifier=Modifier.fillMaxWidth()) {
            Text(text(if(result.error!=null) R.string.photos_retry else R.string.people_merge_matches_retry))
        }
    }
}
