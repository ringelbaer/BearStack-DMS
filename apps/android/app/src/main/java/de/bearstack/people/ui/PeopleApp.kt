package de.bearstack.people.ui

import androidx.compose.foundation.*
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.platform.LocalSoftwareKeyboardController
import androidx.compose.ui.semantics.*
import androidx.compose.ui.text.input.*
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.DialogProperties
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import coil.ImageLoader
import coil.compose.AsyncImage
import de.bearstack.people.statistics.StatisticsPanel
import de.bearstack.people.data.remote.Person
import de.bearstack.people.people.*

@Composable
fun PeopleApp(vm: PeopleViewModel) {
    val state by vm.state.collectAsStateWithLifecycle()
    val dark = isSystemInDarkTheme()
    MaterialTheme(colorScheme=if(dark) darkColorScheme(primary=Color(0xff75d2e8)) else lightColorScheme(primary=Color(0xff146e83))) {
        Surface(Modifier.fillMaxSize()) {
            if (!state.connected) ConnectionScreen(state,vm)
            else LabelingScreen(state,vm)
            state.certificate?.let { certificate ->
                AlertDialog(onDismissRequest={ if(!state.busy) vm.cancelCertificate() },title={ Text("Serverzertifikat prüfen") },
                    text={ Column(Modifier.verticalScroll(rememberScrollState())) {
                        Text("Vergleiche diesen SHA-256-Fingerabdruck mit dem Zertifikat auf deinem BearStack-Server. Erst danach werden Zugangsdaten gesendet.")
                        Spacer(Modifier.height(16.dp)); Text(certificate.fingerprint)
                        Spacer(Modifier.height(16.dp)); Text("${certificate.subject}\nGültig bis ${certificate.expires}")
                    } }, confirmButton={ TextButton(onClick=vm::confirmCertificate,enabled=!state.busy) { Text("Abgeglichen und vertrauen") } },
                    dismissButton={ TextButton(onClick=vm::cancelCertificate,enabled=!state.busy) { Text("Abbrechen") } })
            }
        }
    }
}
@Composable
private fun ConnectionScreen(state: PeopleState, vm: PeopleViewModel) {
    var url by rememberSaveable { mutableStateOf("") }
    var username by rememberSaveable { mutableStateOf("") }
    var password by remember { mutableStateOf("") }
    Column(Modifier.fillMaxSize().safeDrawingPadding().imePadding().verticalScroll(rememberScrollState()).padding(24.dp),
        verticalArrangement=Arrangement.spacedBy(16.dp)) {
        Text("BearStack Personen", style=MaterialTheme.typography.headlineMedium)
        Text("Verbinde dich mit deiner BearStack-Instanz ab Version 0.30.0.")
        OutlinedTextField(url,{url=it},Modifier.fillMaxWidth(),label={Text("HTTPS-Adresse")},singleLine=true,
            enabled=!state.busy,keyboardOptions=KeyboardOptions(keyboardType=KeyboardType.Uri),placeholder={Text("https://bearstack.example/")})
        OutlinedTextField(username,{username=it},Modifier.fillMaxWidth(),label={Text("Benutzername")},singleLine=true,enabled=!state.busy)
        OutlinedTextField(password,{password=it},Modifier.fillMaxWidth(),label={Text("Passwort")},singleLine=true,enabled=!state.busy,
            visualTransformation=PasswordVisualTransformation(),keyboardOptions=KeyboardOptions(keyboardType=KeyboardType.Password))
        Button(onClick={vm.connect(url,username,password)},enabled=!state.busy && url.isNotBlank() && username.isNotBlank() && password.isNotEmpty(),
            modifier=Modifier.fillMaxWidth()) { Text("Verbinden") }
        if (state.busy) LinearProgressIndicator(Modifier.fillMaxWidth())
        state.error?.let { Text(it,color=MaterialTheme.colorScheme.error,modifier=Modifier.semantics { liveRegion=LiveRegionMode.Polite }) }
    }
}
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun LabelingScreen(state: PeopleState, vm: PeopleViewModel) {
    var menu by remember { mutableStateOf(false) }
    var statistics by rememberSaveable { mutableStateOf(false) }
    var held by remember { mutableStateOf<Long?>(null) }
    var accessibleZoom by remember { mutableStateOf(false) }
    val enabled = !state.busy && !state.unresolved && held == null
    LaunchedEffect(state.person?.id,state.person?.revision) { held=null;accessibleZoom=false }
    Box(Modifier.fillMaxSize()) {
        Scaffold(topBar={ TopAppBar(title={Text(if(statistics) "Statistik" else "Personen benennen")},actions={
            TextButton(onClick={menu=true}) { Text("Menü") }
            DropdownMenu(menu,{menu=false}) {
                DropdownMenuItem(text={Text(if(statistics) "Zur Bearbeitung" else "Statistik")},onClick={statistics=!statistics;menu=false},enabled=enabled)
                DropdownMenuItem(text={Text("Gruppe ignorieren")},onClick={vm.ignore();menu=false},enabled=enabled && state.person!=null)
                DropdownMenuItem(text={Text("Gruppe überspringen")},onClick={vm.skip();menu=false},enabled=enabled && state.person!=null)
                DropdownMenuItem(text={Text("Letztes Überspringen zurücknehmen")},onClick={vm.back();menu=false},enabled=enabled && state.canGoBack)
                DropdownMenuItem(text={Text("Übersprungene bearbeiten (${state.skipped})")},onClick={vm.newPass(true);menu=false},enabled=enabled && state.person==null && state.skipped>0)
                DropdownMenuItem(text={Text("Verbindung wechseln")},onClick={vm.switchConnection();menu=false},enabled=!state.busy)
            }
        }) },floatingActionButton={
            if(!statistics && state.person!=null) FloatingActionButton(onClick={if(enabled)vm.startNaming()},
                modifier=Modifier.semantics { contentDescription="Person benennen"; if(!enabled) disabled() }) {
                Text("✎",style=MaterialTheme.typography.headlineMedium)
            }
        }) { padding ->
            PersonSwipeArea(gestureKey=state.person?.let { it.id to it.revision },
                enabled=enabled && !statistics && !menu && !state.naming && (state.person!=null || state.canGoBack),
                onSwipe={when(it) { SwipeAction.Ignore -> vm.ignore(); SwipeAction.Skip -> vm.skip(); SwipeAction.Back -> vm.back() }},
                modifier=Modifier.fillMaxSize().padding(padding).imePadding()) {
                if(state.busy) LinearProgressIndicator(Modifier.fillMaxWidth())
                state.error?.let { error ->
                    Card(colors=CardDefaults.cardColors(containerColor=MaterialTheme.colorScheme.errorContainer)) {
                        Column(Modifier.padding(16.dp)) {
                            Text(error,Modifier.semantics { liveRegion=LiveRegionMode.Polite })
                            TextButton(onClick=vm::retry,enabled=!state.busy) { Text(if(state.unresolved) "Offene Aktion prüfen" else "Erneut versuchen") }
                        }
                    }
                }
                if(statistics) StatisticsPanel(state.stats)
                else state.person?.let { person ->
                    Text("Unbenannte Person",style=MaterialTheme.typography.headlineSmall)
                    Text("${person.offset+1}–${minOf(person.offset+4L,person.count)} von ${person.count} Gesichtern")
                    FaceGrid(person,enabled,vm.images,vm::image,onDetach=vm::detach,
                        onHold={ held=it },onZoom={held=it;accessibleZoom=true})
                    Row(Modifier.fillMaxWidth(),horizontalArrangement=Arrangement.SpaceBetween) {
                        OutlinedButton(onClick={vm.page(-1)},enabled=enabled && person.offset>0) { Text("Zurück") }
                        OutlinedButton(onClick={vm.page(1)},enabled=enabled && person.offset+4<person.count) { Text("Weiter") }
                    }
                    Text("Halten: Originalfoto · Nach oben: ignorieren · Nach links: überspringen · Nach rechts: zurück",style=MaterialTheme.typography.bodySmall)
                    Spacer(Modifier.height(80.dp))
                } ?: run {
                    if(!state.busy && !state.unresolved) {
                        Text("Durchgang abgeschlossen",style=MaterialTheme.typography.headlineSmall)
                        if(state.canGoBack) OutlinedButton(onClick=vm::back,enabled=enabled) { Text("Letztes Überspringen zurücknehmen") }
                        Button(onClick={vm.newPass(false)}) { Text("Neuen Durchgang starten") }
                        if(state.skipped>0) OutlinedButton(onClick={vm.newPass(true)}) { Text("Übersprungene bearbeiten (${state.skipped})") }
                    }
                }
            }
        }
        held?.let { face ->
            var loading by remember(face) { mutableStateOf(true) }
            var failed by remember(face) { mutableStateOf(false) }
            Column(Modifier.fillMaxSize().background(Color.Black.copy(alpha=.94f)).safeDrawingPadding().padding(16.dp),
                horizontalAlignment=Alignment.CenterHorizontally,verticalArrangement=Arrangement.spacedBy(8.dp)) {
                Box(Modifier.weight(1f).fillMaxWidth(),contentAlignment=Alignment.Center) {
                    vm.images?.let { AsyncImage(vm.original(face),"Originalfoto",imageLoader=it,
                        modifier=Modifier.fillMaxSize(),contentScale=ContentScale.Fit,
                        onLoading={loading=true;failed=false},onSuccess={loading=false;failed=false},
                        onError={loading=false;failed=true}) }
                    if(loading) CircularProgressIndicator()
                    if(failed) Text(if(accessibleZoom) "Originalfoto konnte nicht geladen werden. Vorschau schließen und erneut öffnen."
                        else "Originalfoto konnte nicht geladen werden. Loslassen und erneut halten.",
                        color=Color.White,modifier=Modifier.padding(24.dp))
                }
                state.person?.facePaths?.get(face)?.takeIf {it.isNotEmpty()}?.let {
                    Text(it,color=Color.White,style=MaterialTheme.typography.bodySmall,
                        modifier=Modifier.fillMaxWidth().testTag("original-photo-path"))
                }
                if(accessibleZoom) Button(onClick={held=null;accessibleZoom=false}) { Text("Vorschau schließen") }
            }
        }
    }
    if(state.naming) NamingDialog(state,vm,enabled)
    if(state.undoIgnores.isNotEmpty()) key(state.naming) { IgnoreUndoToast(state.undoIgnores.size,vm::undoIgnore) }
}

@Composable
fun FaceGrid(person: Person, enabled: Boolean, images: ImageLoader?, image: (Long, Boolean) -> String?,
    onDetach: (Long) -> Unit, onHold: (Long?) -> Unit, onZoom: (Long) -> Unit) {
    var holding by remember { mutableStateOf(false) }
    val active by rememberUpdatedState(enabled)
    Column(Modifier.fillMaxWidth().testTag("face-grid"),verticalArrangement=Arrangement.spacedBy(8.dp)) {
        person.faces.chunked(2).forEach { row ->
            Row(Modifier.fillMaxWidth(),horizontalArrangement=Arrangement.spacedBy(8.dp)) {
                row.forEach { face ->
                    key(face) {
                        Column(Modifier.weight(1f)) {
                            Box(Modifier.fillMaxWidth().testTag("face-$face").aspectRatio(1f).clip(RoundedCornerShape(16.dp))
                                .background(MaterialTheme.colorScheme.surfaceContainerHighest)
                                .semantics { contentDescription="Gesicht ${person.faces.indexOf(face)+person.offset+1}"
                                    customActions=listOf(CustomAccessibilityAction("Originalfoto anzeigen") { if(active) {onZoom(face);true} else false }) }
                                .pointerInput(face) {
                                    detectTapGestures(onLongPress={if(active){holding=true;onHold(face)}},onPress={
                                        try { tryAwaitRelease() } finally { if(holding){holding=false;onHold(null)} }
                                    })
                                }) {
                                if(images!=null) AsyncImage(image(face,false),null,imageLoader=images,
                                    modifier=Modifier.fillMaxSize(),contentScale=ContentScale.Crop)
                                if(person.count>1) FilledTonalIconButton(onClick={onDetach(face)},enabled=enabled && !holding,
                                    modifier=Modifier.align(Alignment.BottomStart).padding(4.dp).size(48.dp)
                                        .semantics { contentDescription="Dieses Gesicht einzeln benennen" }) { Text("×",style=MaterialTheme.typography.headlineMedium) }
                            }
                            person.facePaths[face]?.takeIf {it.isNotEmpty()}?.let {
                                Text(it,style=MaterialTheme.typography.bodySmall,
                                    modifier=Modifier.fillMaxWidth().padding(top=6.dp).testTag("face-path-$face"))
                            }
                        }
                    }
                }
            }
        }
    }
}
@Composable
private fun NamingDialog(state: PeopleState, vm: PeopleViewModel, enabled: Boolean) {
    val focus = remember { FocusRequester() }
    val keyboard = LocalSoftwareKeyboardController.current
    LaunchedEffect(Unit) { focus.requestFocus(); keyboard?.show() }
    AlertDialog(onDismissRequest=vm::closeNaming,title={Text(if(state.duplicates.isEmpty()) "Person benennen" else "Name bereits vorhanden")},
        properties=DialogProperties(usePlatformDefaultWidth=false),modifier=Modifier.fillMaxWidth().padding(16.dp).imePadding(),
        text={ Column(Modifier.verticalScroll(rememberScrollState()),verticalArrangement=Arrangement.spacedBy(8.dp)) {
            OutlinedTextField(state.name,vm::nameChanged,label={Text("Name")},singleLine=true,enabled=enabled,
                modifier=Modifier.fillMaxWidth().focusRequester(focus),keyboardOptions=KeyboardOptions(imeAction=ImeAction.Done),
                keyboardActions=KeyboardActions(onDone={vm.submitName()}))
            if(state.duplicates.isNotEmpty()) Text("Einer vorhandenen Person zuordnen oder separat mit demselben Namen benennen:")
            (state.duplicates.ifEmpty { state.suggestions }).forEach { person ->
                Surface(onClick={vm.assign(person)},enabled=enabled,shape=RoundedCornerShape(12.dp),color=MaterialTheme.colorScheme.surfaceContainer) {
                    Row(Modifier.fillMaxWidth().padding(8.dp),verticalAlignment=Alignment.CenterVertically,horizontalArrangement=Arrangement.spacedBy(12.dp)) {
                        vm.images?.let { AsyncImage(vm.image(person.faceId),null,imageLoader=it,modifier=Modifier.size(48.dp).clip(RoundedCornerShape(8.dp))) }
                        Column(Modifier.weight(1f)) { Text(person.name);Text("${person.count} Gesichter · #${person.id}",style=MaterialTheme.typography.bodySmall) }
                    }
                }
            }
            state.error?.let { Text(it,color=MaterialTheme.colorScheme.error) }
            if(state.unresolved) {
                TextButton(onClick=vm::retry,enabled=!state.busy) { Text("Offene Aktion prüfen") }
                TextButton(onClick=vm::switchConnection,enabled=!state.busy) { Text("Verbindung prüfen") }
            }
        } },confirmButton={ TextButton(onClick={vm.submitName(state.duplicates.isNotEmpty())},enabled=enabled && state.name.isNotBlank()) {
            Text(if(state.duplicates.isNotEmpty()) "Separat benennen" else "Speichern")
        } },dismissButton={ TextButton(onClick=vm::closeNaming,enabled=enabled) { Text("Abbrechen") } })
}
