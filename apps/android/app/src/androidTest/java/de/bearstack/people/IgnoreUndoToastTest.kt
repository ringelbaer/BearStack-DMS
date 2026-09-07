package de.bearstack.people

import androidx.compose.foundation.layout.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.createComposeRule
import de.bearstack.people.ui.IgnoreUndoToast
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test

class IgnoreUndoToastTest {
    @get:Rule val compose=createComposeRule()
    @Test fun topToastIsClickableAndLeavesUnderlyingControlsUsable() {
        var edits=0;var undos=0
        var visible by mutableStateOf(true)
        compose.setContent {MaterialTheme {
            Box(Modifier.fillMaxSize(),contentAlignment=Alignment.Center) {
                Button(onClick={edits++}) {Text("Nächste Person benennen")}
            }
            if(visible) IgnoreUndoToast(1) {undos++;visible=false}
        }}
        compose.onNodeWithText("Nächste Person benennen").performTouchInput {click()}
        compose.runOnIdle {assertEquals(1,edits);assertEquals(0,undos)}
        compose.onNodeWithTag("ignore-undo-toast").assertIsDisplayed().performTouchInput {click()}
        compose.runOnIdle {assertEquals(1,undos)}
        compose.onNodeWithTag("ignore-undo-toast").assertDoesNotExist()
    }
}
