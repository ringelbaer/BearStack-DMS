package de.bearstack.people

import android.app.Application
import androidx.compose.runtime.*
import androidx.compose.ui.test.*
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.unit.Density
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.lifecycle.ViewModelStore
import androidx.room.Room
import androidx.test.platform.app.InstrumentationRegistry
import de.bearstack.people.data.local.LabelingDatabase
import de.bearstack.people.data.remote.Person
import de.bearstack.people.people.PeopleViewModel
import de.bearstack.people.photos.PhotosMenu
import de.bearstack.people.ui.PeopleApp
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test

class MenuNavigationTest {
    @get:Rule val compose=createComposeRule()

    private fun screen(scale: Float=1f, test: (PeopleViewModel,FakeService)->Unit) {
        val app=InstrumentationRegistry.getInstrumentation().targetContext.applicationContext as Application
        val db=Room.inMemoryDatabaseBuilder(app,LabelingDatabase::class.java).build()
        val api=FakeService().apply {upper=3;people[3]=Person(3,"Anna",1,1,30,listOf(30))}
        val store=ViewModelStore()
        lateinit var vm: PeopleViewModel
        compose.runOnUiThread {vm=PeopleViewModel(app,db,api,api.session);store.put("menus",vm)}
        try {
            compose.setGermanContent {
                val density=LocalDensity.current
                CompositionLocalProvider(LocalDensity provides Density(density.density,scale)) {PeopleApp(vm)}
            }
            idle(vm)
            test(vm,api)
        } finally {compose.runOnUiThread {store.clear()}}
    }
    private fun idle(vm: PeopleViewModel)=compose.waitUntil(10_000) {vm.state.value.connected && !vm.state.value.busy}
    private fun menu()=compose.onNodeWithContentDescription("Weitere Optionen").performClick()

    @Test fun primaryDestinationsAreExclusiveAndSettingsCancelPreservesTheSession()=screen(2f) {vm,api ->
        val person=vm.state.value.person
        for(index in listOf(1,2,0)) {
            compose.onNodeWithTag("people-navigation-$index").performScrollTo().performClick();idle(vm)
            compose.onNodeWithTag("people-navigation-$index").assertIsSelected().assertIsDisplayed()
            assertEquals(index==1,vm.state.value.directory)
            assertEquals(index==2,vm.state.value.mergeReview)
            menu()
            compose.onNode(hasText("Zurück") and hasAnyAncestor(isPopup())).assertDoesNotExist()
            compose.onNodeWithText("Hilfe").performScrollTo().assertIsDisplayed()
            compose.onNodeWithText("Einstellungen").performScrollTo().performClick()
            compose.onNodeWithText("Abmelden und Verbindung ändern").performScrollTo().performClick()
            compose.onNodeWithText("Abbrechen").performClick()
            assertTrue(vm.state.value.connected)
            compose.onNodeWithText("Schließen").performClick()
            assertEquals(0,api.commits)
        }
        assertEquals(person,vm.state.value.person)
    }

    @Test fun statisticsHasNoHiddenPersonActionsAndBackReturnsToNaming()=screen {vm,api ->
        val person=vm.state.value.person
        menu();compose.onNodeWithText("Statistik").performClick()
        compose.onNodeWithTag("labeling-action-bar").assertDoesNotExist()
        compose.onNodeWithTag("people-navigation-0").assertDoesNotExist()
        menu()
        compose.onNodeWithText("Zuordnungen nach Ordner prüfen").assertDoesNotExist()
        compose.onAllNodesWithText("Übersprungene bearbeiten",substring=true).assertCountEquals(0)
        compose.onNodeWithText("Hilfe").performClick()
        compose.onNodeWithText("Die Statistik zeigt",substring=true).assertIsDisplayed()
        compose.onNodeWithText("Schließen").performClick()
        androidx.test.espresso.Espresso.pressBack()
        compose.onNodeWithTag("labeling-action-bar").assertIsDisplayed()
        assertEquals(person,vm.state.value.person);assertEquals(0,api.commits)
    }

    @Test fun selectionClearAndUpNavigationHaveDistinctEffects()=screen {vm,api ->
        compose.onNodeWithTag("people-navigation-1").performScrollTo().performClick();idle(vm)
        compose.onNodeWithText("Anna").performClick();idle(vm)
        compose.onNodeWithTag("select-face-30").performScrollTo().performClick()
        compose.onNodeWithText("Gesichter zuordnen").assertIsDisplayed()
        androidx.test.espresso.Espresso.pressBack()
        assertTrue(vm.state.value.selectedFaces.isEmpty())
        assertEquals(3L,vm.state.value.selectedPerson!!.id)
        compose.onNodeWithTag("select-face-30").performClick()
        compose.onNodeWithContentDescription("Zurück").performClick();idle(vm)
        assertNull(vm.state.value.selectedPerson)
        assertTrue(vm.state.value.selectedFaces.isEmpty())
        assertTrue(vm.state.value.directory);assertEquals(0,api.commits)
    }

    @Test fun localOnlyMenuOffersSetupAndKeepsUnavailableFrameInPlace() {
        var connections=0
        compose.setGermanContent {
            de.bearstack.people.photos.PhotosScreen(null,null,false,onPeople={},onConnection={connections++})
        }
        menu()
        compose.onNodeWithText("Karte").assertDoesNotExist()
        compose.onNodeWithText("Bilderrahmen starten").assertIsDisplayed().assertIsNotEnabled()
        compose.onNode(hasText("Einstellungen") and hasAnyAncestor(isPopup())).performClick()
        compose.onNodeWithText("Abmelden und Verbindung ändern").assertDoesNotExist()
        compose.onNodeWithText("Verbindung einrichten").performClick()
        assertEquals(1,connections)
    }

    @Test fun applicableGalleryActionsRemainVisibleWhileLoading() {
        var loading by mutableStateOf(true)
        var visits=0
        compose.setGermanContent {
            PhotosMenu(onSettings={},onMap={visits++},onFrame={visits++},onPeople=null,
                onDirectoryPeople={visits++},actionsEnabled=!loading)
        }
        menu()
        for(label in listOf("Personen im Ordner","Karte","Bilderrahmen starten"))
            compose.onNodeWithText(label).assertIsDisplayed().assertIsNotEnabled()
        compose.onNodeWithText("Personen verwalten").assertDoesNotExist()
        compose.onNodeWithText("Einstellungen").assertIsEnabled()
        compose.runOnIdle {loading=false}
        compose.onNodeWithText("Personen im Ordner").assertIsEnabled().performClick()
        assertEquals(1,visits)
    }
}
