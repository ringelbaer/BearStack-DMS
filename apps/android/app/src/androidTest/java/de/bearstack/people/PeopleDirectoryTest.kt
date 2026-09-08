package de.bearstack.people

import android.app.Application
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.semantics.SemanticsActions
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

class PeopleDirectoryTest {
    @get:Rule val compose=createComposeRule()

    private fun screen(scale: Float=1f,test: (PeopleViewModel,FakeService)->Unit) {
        val app=InstrumentationRegistry.getInstrumentation().targetContext.applicationContext as Application
        val db=Room.inMemoryDatabaseBuilder(app,LabelingDatabase::class.java).build()
        val service=FakeService().apply { upper=3;people[3]=Person(3,"Anna",1,1,30,listOf(30),facePaths=mapOf(30L to "Fotos / Urlaub / Anna.jpg")) }
        val store=ViewModelStore()
        lateinit var vm: PeopleViewModel
        compose.runOnUiThread { vm=PeopleViewModel(app,db,service,service.session);store.put("test",vm) }
        try {
            compose.setContent {
                val density=LocalDensity.current
                CompositionLocalProvider(LocalDensity provides Density(density.density,scale)) {PeopleApp(vm)}
            }
            idle(vm)
            compose.onNodeWithText("Menü").performClick()
            compose.onNodeWithText("Personen").performClick()
            idle(vm)
            compose.onNodeWithText("Anna").performClick()
            idle(vm)
            test(vm,service)
        } finally { compose.runOnUiThread {store.clear()} }
    }
    private fun idle(vm: PeopleViewModel) = compose.waitUntil(10_000) {vm.state.value.connected && !vm.state.value.busy}

    @Test fun listRenameFavoriteAndRemoveLastFace() = screen(2f) {vm,api ->
        compose.onNodeWithText("Person umbenennen").performScrollTo().performClick()
        compose.onNodeWithText("Name").performTextReplacement("Anna Neu")
        compose.onNodeWithText("Speichern").performClick();idle(vm)
        compose.onNodeWithText("Anna Neu").assertExists()
        compose.onNodeWithText("Galeriesuche im Browser").performScrollTo().assertIsEnabled()
        compose.onNodeWithContentDescription("Bild favorisieren").performScrollTo().performClick();idle(vm)
        compose.onNodeWithContentDescription("Favorisierung aufheben").assertExists()
        compose.onNodeWithContentDescription("Favorisierung aufheben").performClick();idle(vm)
        compose.onNodeWithContentDescription("Bild favorisieren").assertExists()
        compose.onNodeWithContentDescription("Zuordnung entfernen").performClick();idle(vm)
        compose.onNodeWithText("Noch keine benannten Personen vorhanden.").assertExists()
        assertEquals(4,api.commits);assertEquals("",api.people.getValue(4).name)
        compose.onNodeWithText("Zurück").performClick();idle(vm)
        compose.onNodeWithText("Personen benennen").assertExists()
        assertEquals(1L,vm.state.value.person!!.id)
    }

    @Test fun holdSwipeReleaseAndAccessiblePreviewNeverEdit() = screen {vm,api ->
        val face=compose.onNodeWithTag("face-30").performScrollTo()
        face.performTouchInput {down(center)}
        compose.mainClock.advanceTimeBy(800)
        compose.onNodeWithTag("original-photo-path").assertIsDisplayed()
        face.performTouchInput {moveBy(Offset(0f,100f));moveBy(Offset(0f,-200f));up()}
        compose.onNodeWithTag("original-photo-path").assertDoesNotExist()
        face.performTouchInput {down(center)}
        compose.mainClock.advanceTimeBy(800)
        face.performTouchInput {cancel()}
        compose.onNodeWithTag("original-photo-path").assertDoesNotExist()
        val action=face.fetchSemanticsNode().config[SemanticsActions.CustomActions].single {it.label=="Originalfoto anzeigen"}
        compose.runOnIdle {assertTrue(action.action())}
        compose.onNodeWithText("Vorschau schließen").performClick()
        compose.onNodeWithTag("original-photo-path").assertDoesNotExist()
        assertEquals(0,api.commits);assertEquals(3L,vm.state.value.selectedPerson!!.id)
    }
}
