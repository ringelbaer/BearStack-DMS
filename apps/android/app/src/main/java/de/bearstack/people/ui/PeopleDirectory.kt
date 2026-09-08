package de.bearstack.people.ui

import android.content.ActivityNotFoundException
import android.content.Intent
import android.net.Uri
import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.*
import androidx.compose.ui.unit.dp
import coil.compose.AsyncImage
import de.bearstack.people.people.PeopleState
import de.bearstack.people.people.PeopleViewModel

@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun PeopleDirectoryScreen(state: PeopleState, vm: PeopleViewModel) {
    val person=state.selectedPerson
    var held by remember { mutableStateOf<Long?>(null) }
    var heldDismissed by remember { mutableStateOf(false) }
    var accessible by remember { mutableStateOf(false) }
    var zoom by remember { mutableFloatStateOf(0f) }
    var browserError by remember(person?.id) { mutableStateOf<String?>(null) }
    val context=LocalContext.current
    val zoomDistance=with(LocalDensity.current) { 240.dp.toPx() }
    val zoomDrag: (Float) -> Unit = { zoom=zoomAfterDrag(zoom,it,zoomDistance) }
    val enabled=!state.busy && !state.unresolved && held==null
    LaunchedEffect(person?.id,person?.revision,person?.offset) { held=null;accessible=false }
    BackHandler {
        if(held!=null) { if(accessible) held=null else heldDismissed=true }
        else if(state.naming) vm.closeNaming()
        else if(person!=null) vm.closePerson() else vm.closeDirectory()
    }
    Box(Modifier.fillMaxSize()) {
        Scaffold(topBar={ TopAppBar(title={Text("Personen")},navigationIcon={
            TextButton(onClick={if(person!=null) vm.closePerson() else vm.closeDirectory()},enabled=enabled) { Text("Zurück") }
        },actions={
            if(state.error!=null) TextButton(onClick=vm::switchConnection,enabled=!state.busy && held==null) { Text("Verbindung") }
        }) }) { padding ->
            Column(Modifier.fillMaxSize().padding(padding).imePadding()) {
                Box(Modifier.fillMaxWidth().height(4.dp)) { if(state.busy) LinearProgressIndicator(Modifier.fillMaxSize()) }
                state.error?.let { error ->
                    Card(Modifier.fillMaxWidth().padding(12.dp),colors=CardDefaults.cardColors(containerColor=MaterialTheme.colorScheme.errorContainer)) {
                        Column(Modifier.padding(12.dp)) {
                            Text(error,Modifier.semantics { liveRegion=LiveRegionMode.Polite })
                            TextButton(onClick=vm::retry,enabled=!state.busy && held==null) {
                                Text(if(state.unresolved) "Offene Aktion prüfen" else "Erneut versuchen")
                            }
                        }
                    }
                }
                if(person==null) {
                    LazyColumn(Modifier.fillMaxSize().testTag("named-people"),contentPadding=PaddingValues(16.dp),verticalArrangement=Arrangement.spacedBy(12.dp)) {
                        item {
                            Row(Modifier.fillMaxWidth(),verticalAlignment=Alignment.CenterVertically) {
                                Text("Benannte Personen",Modifier.weight(1f),style=MaterialTheme.typography.titleLarge)
                                TextButton(onClick=vm::openDirectory,enabled=enabled) { Text("Aktualisieren") }
                            }
                        }
                        items(state.namedPeople,key={it.id}) { p ->
                            Surface(onClick={vm.openPerson(p)},enabled=enabled,shape=RoundedCornerShape(16.dp),color=MaterialTheme.colorScheme.surfaceContainer) {
                                Row(Modifier.fillMaxWidth().padding(12.dp),verticalAlignment=Alignment.CenterVertically,horizontalArrangement=Arrangement.spacedBy(16.dp)) {
                                    Box(Modifier.size(72.dp).clip(RoundedCornerShape(12.dp)).background(MaterialTheme.colorScheme.surfaceContainerHighest)) {
                                        vm.images?.let { AsyncImage(vm.image(p.faceId),null,imageLoader=it,modifier=Modifier.fillMaxSize()) }
                                    }
                                    Column(Modifier.weight(1f)) {
                                        Text(p.name,style=MaterialTheme.typography.titleMedium)
                                        Text("${p.count} Gesichter · #${p.id}",style=MaterialTheme.typography.bodySmall)
                                    }
                                }
                            }
                        }
                        if(state.namedPeople.isEmpty() && !state.busy && state.error==null) item { Text("Noch keine benannten Personen vorhanden.") }
                        if(state.namedHasNext) item { OutlinedButton(onClick=vm::moreNamedPeople,enabled=enabled,modifier=Modifier.fillMaxWidth()) { Text("Weitere Personen laden") } }
                    }
                } else key(person.id) {
                    Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(16.dp),verticalArrangement=Arrangement.spacedBy(12.dp)) {
                        Text(person.name,style=MaterialTheme.typography.headlineSmall)
                        Text("${person.offset+1}–${minOf(person.offset+4L,person.count)} von ${person.count} Gesichtern")
                        OutlinedButton(onClick=vm::startNaming,enabled=enabled) { Text("Person umbenennen") }
                        OutlinedButton(onClick={
                            vm.gallery(person.name)?.let { url ->
                                try {
                                    context.startActivity(Intent(Intent.ACTION_VIEW,Uri.parse(url)).addCategory(Intent.CATEGORY_BROWSABLE))
                                    browserError=null
                                } catch(_: ActivityNotFoundException) { browserError="Kein Browser zum Öffnen der Galeriesuche verfügbar." }
                            }
                        },enabled=enabled) { Text("Galeriesuche im Browser") }
                        browserError?.let { Text(it,color=MaterialTheme.colorScheme.error) }
                        FaceGrid(person,enabled,vm.images,vm::image,onDetach=vm::unassign,
                            onHold={held=it;heldDismissed=false;zoom=0f},
                            onZoom={held=it;heldDismissed=false;zoom=0f;accessible=true},
                            onZoomDrag=zoomDrag,managing=true,onFavorite=vm::favorite)
                        Row(Modifier.fillMaxWidth(),horizontalArrangement=Arrangement.SpaceBetween) {
                            OutlinedButton(onClick={vm.personPage(-1)},enabled=enabled && person.offset>0) { Text("Vorherige Bilder") }
                            OutlinedButton(onClick={vm.personPage(1)},enabled=enabled && person.offset+4<person.count) { Text("Weitere Bilder") }
                        }
                        Text("×: Zuordnung entfernen und wieder unbenannt bereitstellen. Die Bilddatei bleibt erhalten. ★: Als Vergleichsbild favorisieren. Halten: Originalfoto; dabei runterwischen zum Vergrößern, hoch zum Verkleinern.",style=MaterialTheme.typography.bodySmall)
                    }
                }
            }
        }
        held?.takeUnless { heldDismissed }?.let { face ->
            Column(Modifier.fillMaxSize().background(Color.Black.copy(alpha=.94f)).safeDrawingPadding().padding(16.dp),
                horizontalAlignment=Alignment.CenterHorizontally,verticalArrangement=Arrangement.spacedBy(8.dp)) {
                vm.images?.let { images ->
                    OriginalPhoto(vm.original(face),images,person?.faceBounds?.get(face),zoom,onZoom={zoom=it},onDrag=zoomDrag,
                        modifier=Modifier.weight(1f).fillMaxWidth(),onNewTouch=if(accessible) null else { {heldDismissed=true} })
                }
                person?.facePaths?.get(face)?.takeIf { it.isNotEmpty() }?.let {
                    Text(it,color=Color.White,style=MaterialTheme.typography.bodySmall,modifier=Modifier.fillMaxWidth().testTag("original-photo-path"))
                }
                if(accessible) Button(onClick={held=null;accessible=false}) { Text("Vorschau schließen") }
            }
        }
    }
    if(state.naming) NamingDialog(state,vm,enabled)
}
