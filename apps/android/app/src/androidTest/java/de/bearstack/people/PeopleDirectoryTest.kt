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

    private fun screen(scale: Float=1f, openDetail: Boolean=true, setup: (FakeService)->Unit = {}, test: (PeopleViewModel,FakeService)->Unit) {
        val app=InstrumentationRegistry.getInstrumentation().targetContext.applicationContext as Application
        val db=Room.inMemoryDatabaseBuilder(app,LabelingDatabase::class.java).build()
        val service=FakeService().apply { upper=3;people[3]=Person(3,"Anna",1,1,30,listOf(30),facePaths=mapOf(30L to "Fotos / Urlaub / Anna.jpg")) }
        setup(service)
        val store=ViewModelStore()
        lateinit var vm: PeopleViewModel
        compose.runOnUiThread { vm=PeopleViewModel(app,db,service,service.session);store.put("test",vm) }
        try {
            compose.setGermanContent {
                val density=LocalDensity.current
                CompositionLocalProvider(LocalDensity provides Density(density.density,scale)) {PeopleApp(vm)}
            }
            idle(vm)
            compose.onNodeWithText("Menü").performClick()
            compose.onNodeWithText("Personen").performClick()
            idle(vm)
            if(openDetail) {compose.onNodeWithText("Anna").performClick();idle(vm)}
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
        compose.onNodeWithText("Zuordnung entfernen?").assertIsDisplayed()
        assertEquals(3,api.commits)
        compose.onNodeWithText("Abbrechen").performClick()
        assertEquals(3,api.commits)
        compose.onNodeWithContentDescription("Zuordnung entfernen").performClick()
        compose.onNodeWithText("Entfernen").performClick();idle(vm)
        compose.onNodeWithText("Noch keine benannten Personen vorhanden.").assertExists()
        assertEquals(4,api.commits);assertEquals("",api.people.getValue(4).name)
        compose.onNodeWithText("Zurück").performClick();idle(vm)
        compose.onNodeWithText("Personen benennen").assertExists()
        assertEquals(1L,vm.state.value.person!!.id)
    }

    @Test fun searchFindsPeopleBeyondLoadedListAndCanBeCleared() = screen(openDetail=false,setup={api ->
        api.upper=100
        for(id in 4L..80L) api.people[id]=Person(id,"Person $id",1,1,id*10,listOf(id*10))
        api.people[100]=Person(100,"Gesuchte Person",1,1,1000,listOf(1000))
    }) {vm,api ->
        compose.onNodeWithText("Personen suchen").performTextInput("Gesuchte")
        compose.waitUntil(10000) {!vm.state.value.busy && vm.state.value.loadedNamedQuery=="Gesuchte"}
        compose.onNodeWithText("Gesuchte Person").assertIsDisplayed()
        assertEquals(listOf(100L),vm.state.value.namedPeople.map {it.id})
        compose.onNodeWithText("Gesuchte Person").performClick();idle(vm)
        compose.onNodeWithText("Person umbenennen").performClick()
        compose.onNodeWithText("Name").performTextReplacement("Anderer Name")
        compose.onNodeWithText("Speichern").performClick();idle(vm)
        compose.onNodeWithText("Zurück").performClick();idle(vm)
        compose.onNodeWithText("Keine Personen gefunden.").assertIsDisplayed()
        compose.onNodeWithText("Löschen").performClick()
        compose.waitUntil(10000) {!vm.state.value.busy && vm.state.value.loadedNamedQuery.isEmpty()}
        compose.onNodeWithText("Anna").assertIsDisplayed()
        assertTrue(api.directoryQueries.contains("Gesuchte"))
    }

    @Test fun scrollingLoadsMoreFacesAndFavoritePreservesScrollPosition() = screen(setup={api ->
        api.people[3]=api.people.getValue(3).copy(count=85,faces=(30L..114L).toList())
    }) {vm,api ->
        val grid=compose.onNodeWithTag("person-faces")
        grid.performScrollToIndex(38)
        compose.waitUntil(10000) {!vm.state.value.busy && vm.state.value.selectedPerson!!.faces.size>=80}
        grid.performScrollToIndex(70)
        val face=compose.onNodeWithTag("face-99")
        face.assertIsDisplayed()
        val before=face.getUnclippedBoundsInRoot()
        compose.onNode(hasContentDescription("Bild favorisieren") and hasAnyAncestor(hasTestTag("face-99"))).performClick()
        idle(vm)
        assertEquals(before,face.getUnclippedBoundsInRoot())
        assertTrue(99L in vm.state.value.selectedPerson!!.favorites)
        assertTrue(vm.state.value.selectedPerson!!.faces.size>=80)
        grid.performScrollToIndex(78)
        compose.waitUntil(10000) {!vm.state.value.busy && vm.state.value.selectedPerson!!.faces.size==85}
        grid.performScrollToIndex(86)
        compose.onNodeWithText("Alle Bilder geladen.").assertIsDisplayed()
        compose.onNodeWithText("Weitere Bilder").assertDoesNotExist()
        assertEquals(1,api.commits)
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
