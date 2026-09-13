package de.bearstack.people.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.*
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import de.bearstack.people.R

@Composable
internal fun StartupScreen(onDevice: () -> Unit) {
    val colors = MaterialTheme.colorScheme
    Box(Modifier.fillMaxSize().background(Brush.verticalGradient(listOf(colors.primaryContainer, colors.background)))) {
        Column(Modifier.fillMaxSize().safeDrawingPadding().verticalScroll(rememberScrollState()).padding(32.dp)
            .testTag("connection-splash"), horizontalAlignment=Alignment.CenterHorizontally,
            verticalArrangement=Arrangement.Center) {
            Surface(shape=RoundedCornerShape(36.dp), color=colors.surface, shadowElevation=6.dp) {
                Icon(painterResource(R.drawable.ic_bearstack), null, Modifier.size(144.dp), tint=colors.primary)
            }
            Spacer(Modifier.height(28.dp))
            Text(stringResource(R.string.app_name), style=MaterialTheme.typography.headlineLarge, textAlign=TextAlign.Center)
            Spacer(Modifier.height(12.dp))
            Text(stringResource(R.string.connection_loading), style=MaterialTheme.typography.titleMedium, textAlign=TextAlign.Center)
            Spacer(Modifier.height(24.dp))
            CircularProgressIndicator(Modifier.size(28.dp), strokeWidth=3.dp)
            Spacer(Modifier.height(32.dp))
            Text(stringResource(R.string.connection_local_help), style=MaterialTheme.typography.bodyMedium, textAlign=TextAlign.Center)
            Spacer(Modifier.height(12.dp))
            OutlinedButton(onClick=onDevice) { Text(stringResource(R.string.connection_local_photos)) }
        }
    }
}
