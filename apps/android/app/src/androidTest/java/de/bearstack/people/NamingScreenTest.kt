package de.bearstack.people

import android.app.Application
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.unit.Density
import androidx.lifecycle.ViewModelStore
import androidx.room.Room
import androidx.test.platform.app.InstrumentationRegistry
import de.bearstack.people.data.local.LabelingDatabase
import de.bearstack.people.data.remote.Person
import de.bearstack.people.people.PeopleViewModel
import de.bearstack.people.ui.PeopleApp
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test

class NamingScreenTest {
    @get:Rule val compose=createComposeRule()
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
