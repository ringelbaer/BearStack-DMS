package de.bearstack.people.photos

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
import de.bearstack.people.R
import de.bearstack.people.text.uiStrings
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneOffset
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

@Composable internal fun PhotoDateAction(controller: PhotosController, state: PhotosState) {
    var open by rememberSaveable { mutableStateOf(false) }
    val lifecycle = LocalLifecycleOwner.current.lifecycle
    DisposableEffect(controller, lifecycle) {
        val observer = LifecycleEventObserver { _, event ->
            if(event == Lifecycle.Event.ON_STOP) { open = false; controller.cancelDateJump() }
        }
        lifecycle.addObserver(observer)
        onDispose { lifecycle.removeObserver(observer); controller.cancelDateJump() }
    }
    IconButton(onClick = { open = true }, enabled = !state.loading && !state.dateLoading) {
        Icon(painterResource(R.drawable.ic_calendar), stringResource(R.string.photos_date_choose))
    }
    if(open) {
        val path = controller.gridPosition?.key?.substringAfter(':')
        val visible = state.media.firstOrNull { it.path == path } ?: state.media.firstOrNull()
        val initial = runCatching { LocalDate.parse(visible?.date?.take(10)) }.getOrNull() ?: LocalDate.now()
        PhotoDatePicker(initial, onDismiss = { open = false }) {
            open = false
            controller.jumpToDate(it)
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable internal fun PhotoDatePicker(initial: LocalDate, onDismiss: () -> Unit, onSelect: (LocalDate) -> Unit) {
    val picker = rememberDatePickerState(initialSelectedDateMillis = initial.atStartOfDay().toInstant(ZoneOffset.UTC).toEpochMilli(),
        yearRange = 1..9999)
    DatePickerDialog(onDismissRequest = onDismiss, confirmButton = {
        TextButton(enabled = picker.selectedDateMillis != null, onClick = {
            picker.selectedDateMillis?.let { onSelect(Instant.ofEpochMilli(it).atOffset(ZoneOffset.UTC).toLocalDate()) }
        }) { Text(stringResource(R.string.photos_date_jump)) }
    }, dismissButton = {
        TextButton(onClick = onDismiss) { Text(stringResource(R.string.photos_cancel)) }
    }) {
        Column(Modifier.verticalScroll(rememberScrollState())) {
            DatePicker(picker, title = {
                Text(stringResource(R.string.photos_date_choose), Modifier.padding(start = 24.dp, top = 16.dp))
            })
            Text(stringResource(R.string.photos_date_hint), Modifier.padding(horizontal = 24.dp, vertical = 8.dp),
                style = MaterialTheme.typography.bodySmall)
        }
    }
}

@Composable internal fun PhotoDateStatus(controller: PhotosController, state: PhotosState) {
    if(!state.dateLoading && state.dateError == null) return
    val locale = LocalConfiguration.current.locales[0]
    val date = state.jumpDate?.let { runCatching { LocalDate.parse(it) }.getOrNull() }
    Surface(color = if(state.dateError != null) MaterialTheme.colorScheme.errorContainer else MaterialTheme.colorScheme.surfaceContainer) {
        Row(Modifier.fillMaxWidth().padding(horizontal = 12.dp), verticalAlignment = Alignment.CenterVertically) {
            if(state.dateLoading) {
                CircularProgressIndicator(Modifier.size(24.dp), strokeWidth = 2.dp)
                Spacer(Modifier.width(12.dp))
                Text(stringResource(R.string.photos_date_loading,
                    date?.format(DateTimeFormatter.ofLocalizedDate(FormatStyle.MEDIUM).withLocale(locale)).orEmpty()),
                    Modifier.weight(1f), style = MaterialTheme.typography.bodySmall)
            } else {
                Text(uiStrings()(state.dateError!!), Modifier.weight(1f), style = MaterialTheme.typography.bodySmall)
                TextButton(onClick = { date?.let(controller::jumpToDate) }) { Text(stringResource(R.string.photos_retry)) }
            }
            TextButton(onClick = controller::cancelDateJump) {
                Text(stringResource(if(state.dateLoading) R.string.photos_cancel else R.string.photos_close))
            }
        }
    }
}
