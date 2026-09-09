package de.bearstack.people.ui

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.*
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color

@Composable
internal fun BearStackTheme(dark: Boolean = isSystemInDarkTheme(), content: @Composable () -> Unit) {
    val colors = if (dark) darkColorScheme(
        primary=Color(0xff75d2e8), onPrimary=Color(0xff003642),
        primaryContainer=Color(0xff124b59), onPrimaryContainer=Color(0xffc4f2ff),
        secondaryContainer=Color(0xff374b50), onSecondaryContainer=Color(0xffd3e8ee),
        background=Color(0xff141313), surface=Color(0xff141313),
        surfaceContainerLow=Color(0xff1c1b1b), surfaceContainer=Color(0xff242222),
        surfaceContainerHigh=Color(0xff2d2b2b), surfaceContainerHighest=Color(0xff383535),
    ) else lightColorScheme(
        primary=Color(0xff146e83), onPrimary=Color.White,
        primaryContainer=Color(0xffc4edf7), onPrimaryContainer=Color(0xff064b5b),
        secondaryContainer=Color(0xffdae9ed), onSecondaryContainer=Color(0xff243f46),
        background=Color(0xfffffbf8), surface=Color(0xfffffbf8),
        surfaceContainerLow=Color(0xfff8f3ef), surfaceContainer=Color(0xfff1ece8),
        surfaceContainerHigh=Color(0xffebe6e2), surfaceContainerHighest=Color(0xffe5e0dc),
    )
    MaterialTheme(colorScheme=colors, content=content)
}
