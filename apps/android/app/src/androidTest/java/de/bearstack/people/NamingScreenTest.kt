package de.bearstack.people

import android.app.Application
import android.view.WindowInsets
import android.view.inspector.WindowInspector
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.unit.Density
import androidx.lifecycle.ViewModelStore
import androidx.room.Room
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.filters.SdkSuppress
import de.bearstack.people.data.local.LabelingDatabase
import de.bearstack.people.data.remote.Person
import de.bearstack.people.people.PeopleViewModel
import de.bearstack.people.ui.PeopleApp
import kotlinx.coroutines.delay
import kotlinx.coroutines.runBlocking
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test

class NamingScreenTest {
    @get:Rule val compose=createComposeRule()
    @Test @SdkSuppress(minSdkVersion=30)
    fun expiringToastsKeepDialogFocusKeyboardAndDraft() {
        val app=InstrumentationRegistry.getInstrumentation().targetContext.applicationContext as Application
        val db=Room.inMemoryDatabaseBuilder(app,LabelingDatabase::class.java).build()
        val api=FakeService().apply {upper=3;people[3]=Person(3,"",1,1,30,listOf(30))}
        val store=ViewModelStore()
        lateinit var vm: PeopleViewModel
        compose.runOnUiThread {vm=PeopleViewModel(app,db,api,api.session);store.put("test",vm)}
        fun keyboardVisible() = compose.runOnIdle {
            WindowInspector.getGlobalWindowViews().any {
                it.hasWindowFocus() && it.rootWindowInsets?.isVisible(WindowInsets.Type.ime())==true
            }
        }
        try {
            compose.setContent {PeopleApp(vm)}
            compose.waitUntil(10_000){!vm.state.value.busy && vm.state.value.person!=null}
            compose.runOnIdle {vm.ignore()}
            compose.waitUntil(10_000){!vm.state.value.busy && vm.state.value.person?.id==2L}
            // Separate the deadlines so both the count change and final disappearance are observed.
            runBlocking {delay(1500)}
            compose.runOnIdle {vm.ignore()}
            compose.waitUntil(10_000){!vm.state.value.busy && vm.state.value.person?.id==3L}
            compose.onNodeWithContentDescription("Person benennen").performClick()
            val field=compose.onNodeWithText("Name")
            field.performTextInput("Ann")
            compose.onNodeWithTag("ignore-undo-toast").assertIsDisplayed()
            compose.waitUntil(10_000){keyboardVisible()}
            val dialog=compose.onNode(isDialog()).fetchSemanticsNode().id
            for(remaining in listOf(1,0)) {
                compose.waitUntil(10_000){vm.state.value.undoIgnores.size==remaining}
                field.assertIsEnabled().assertIsFocused()
                assertEquals(dialog,compose.onNode(isDialog()).fetchSemanticsNode().id)
                assertTrue("The keyboard closed when the undo toast changed",keyboardVisible())
                compose.runOnIdle {
                    assertTrue(vm.state.value.naming);assertFalse(vm.state.value.busy)
                    assertEquals("Ann",vm.state.value.name);assertEquals(0,api.commits)
                    assertEquals(3L,vm.state.value.person!!.id)
                }
            }
            compose.onNodeWithTag("ignore-undo-toast").assertDoesNotExist()
            field.performTextInput("a")
            compose.runOnIdle {assertEquals("Anna",vm.state.value.name)}
            compose.onNodeWithText("Abbrechen").performClick()
            compose.waitUntil(10_000){api.commits==2 && !vm.state.value.busy}
            assertEquals(setOf(1L,2L),api.receipts.values.map {it.source}.toSet())
            assertTrue(api.receipts.values.all {it.action=="ignore"})
        } finally {compose.runOnUiThread {store.clear()}}
    }
    @Test fun nameDialogAndDuplicateAssignmentWorkWithLargeFontAndKeyboard() {
        val app=InstrumentationRegistry.getInstrumentation().targetContext.applicationContext as Application
        val db=Room.inMemoryDatabaseBuilder(app,LabelingDatabase::class.java).build()
        val api=FakeService().apply {people[9]=Person(9,"Anna",1,1,90,listOf(90))}
        val store=ViewModelStore()
        lateinit var vm: PeopleViewModel
        compose.runOnUiThread {vm=PeopleViewModel(app,db,api,api.session);store.put("test",vm)}
        try {
            compose.setContent {
                val density=LocalDensity.current
                CompositionLocalProvider(LocalDensity provides Density(density.density,2f)) {PeopleApp(vm)}
            }
            compose.waitUntil(10_000){!vm.state.value.busy && vm.state.value.person!=null}
            compose.onNodeWithContentDescription("Person benennen").performClick()
            compose.onNodeWithText("Name",useUnmergedTree=false).performTextInput("Anna")
            compose.waitUntil(10_000){vm.state.value.suggestions.isNotEmpty()}
            compose.onNodeWithText("Speichern").performClick()
            compose.waitUntil(10_000){vm.state.value.duplicates.isNotEmpty()}
            compose.onNodeWithText("Name bereits vorhanden").assertIsDisplayed()
            compose.onNodeWithText("Separat benennen").assertIsDisplayed()
            compose.onNodeWithText("1 Gesichter · #9").performScrollTo().performClick()
            compose.waitUntil(10_000){api.commits==1 && !vm.state.value.busy}
            assertFalse(vm.state.value.naming)
            assertEquals(2L,vm.state.value.person!!.id)
        } finally {compose.runOnUiThread {store.clear()}}
    }
}
