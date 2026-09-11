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
import de.bearstack.people.data.remote.*
import de.bearstack.people.people.PeopleViewModel
import de.bearstack.people.ui.PeopleApp
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.flow.first
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test

class MergeReviewTest {
    @get:Rule val compose=createComposeRule()
    private fun idle(vm: PeopleViewModel) = compose.waitUntil(10_000) {vm.state.value.connected && !vm.state.value.busy}
    private fun screen(scale: Float=1f, setup: (FakeService)->Unit = {}, test: (PeopleViewModel,FakeService,LabelingDatabase)->Unit) {
        val app=InstrumentationRegistry.getInstrumentation().targetContext.applicationContext as Application
        val db=Room.inMemoryDatabaseBuilder(app,LabelingDatabase::class.java).build()
        val api=FakeService().apply {
            upper=6
            for(id in 3L..6L) people[id]=Person(id,if(id%2==0L) "Person $id" else "",1,1,id*10,listOf(id*10),
                facePaths=mapOf(id*10 to "Fotos / Urlaub / Bild$id.jpg"),faceBounds=mapOf(id*10 to FaceBounds(.2f,.2f,.3f,.3f)))
            mergePairs+=MergeSuggestion(1,people.getValue(3),people.getValue(4),.5234)
            mergePairs+=MergeSuggestion(2,people.getValue(5),people.getValue(6),.6789)
        }
        setup(api)
        val store=ViewModelStore()
        lateinit var vm: PeopleViewModel
        compose.runOnUiThread {vm=PeopleViewModel(app,db,api,api.session);store.put("test",vm)}
        try {
            compose.setGermanContent {
                val density=LocalDensity.current
                CompositionLocalProvider(LocalDensity provides Density(density.density,scale)) {PeopleApp(vm)}
            }
            idle(vm)
            compose.runOnUiThread {vm.page(1)};idle(vm)
            compose.onNodeWithText("Menü").performClick()
            compose.onNodeWithText("Ähnliche Gruppen").performClick();idle(vm)
            test(vm,api,db)
        } finally {compose.runOnUiThread {store.clear()}}
    }

    private fun unnamed(api: FakeService) {
        api.people[4]=api.people.getValue(4).copy(name="")
        api.mergePairs[0]=api.mergePairs[0].copy(target=api.people.getValue(4))
    }
    private fun named(api: FakeService) {
        api.people[3]=api.people.getValue(3).copy(name="Ada")
        api.people[4]=api.people.getValue(4).copy(name="Grace")
        api.mergePairs[0]=api.mergePairs[0].copy(source=api.people.getValue(3),target=api.people.getValue(4))
    }
    @Test fun namedMergeRequiresConfirmationAndCancelDoesNotWrite() = screen(2f,setup=::named) {vm,api,db ->
        val before=api.people.toMap()
        compose.onNodeWithText("Zusammenführen").performClick()
        compose.onNodeWithText("Benannte Gruppen zusammenführen?").assertIsDisplayed()
        compose.onNodeWithText("Die Gruppen „Ada“ und „Grace“ sind bereits benannt. Alle Gesichter von „Ada“ werden „Grace“ zugeordnet. Der Name „Grace“ bleibt erhalten.").assertIsDisplayed()
        assertEquals(0,api.commits)
        assertNull(runBlocking {db.dao().pending(api.session.scope)})
        compose.onNodeWithText("Abbrechen").performClick()
        assertEquals(before,api.people);assertEquals(0,api.commits)
        assertEquals(1L,vm.state.value.mergeSuggestion!!.id)
        compose.onNodeWithText("Zusammenführen").performClick()
        compose.onNodeWithText("Trotzdem zusammenführen").performClick();idle(vm)
        assertEquals(1,api.commits)
        assertFalse(api.people.containsKey(3))
        assertEquals("Grace",api.people.getValue(4).name)
        assertEquals(2L,api.people.getValue(4).count)
        assertEquals(2L,vm.state.value.mergeSuggestion!!.id)
    }
    @Test fun equalNamesStillRequireConfirmationAndRejectNeedsNone() = screen(setup={api ->
        named(api)
        api.people[3]=api.people.getValue(3).copy(name="Grace")
        api.mergePairs[0]=api.mergePairs[0].copy(source=api.people.getValue(3))
    }) {vm,api,_ ->
        compose.onNodeWithText("Zusammenführen").performClick()
        compose.onNodeWithText("Benannte Gruppen zusammenführen?").assertIsDisplayed()
        compose.onNodeWithText("Abbrechen").performClick()
        compose.onNodeWithText("Getrennt lassen").performClick();idle(vm)
        compose.onNodeWithText("Benannte Gruppen zusammenführen?").assertDoesNotExist()
        assertEquals(1,api.commits)
        assertEquals("reject_merge",api.receipts.values.single().action)
        assertTrue(api.people.containsKey(3));assertEquals(1L,api.people.getValue(4).count)
    }
    @Test fun namedMergeConflictNeedsNewConfirmationWithCurrentNames() = screen(setup=::named) {vm,api,_ ->
        compose.onNodeWithText("Zusammenführen").performClick()
        api.people[4]=api.people.getValue(4).copy(revision=2,name="Grace geändert")
        api.mergePairs[0]=api.mergePairs[0].copy(target=api.people.getValue(4))
        compose.onNodeWithText("Trotzdem zusammenführen").performClick();idle(vm)
        assertEquals(0,api.commits)
        compose.onNodeWithText("Benannte Gruppen zusammenführen?").assertDoesNotExist()
        compose.onNodeWithText("Zusammenführen").performClick()
        compose.onNodeWithText("Die Gruppen „Ada“ und „Grace geändert“ sind bereits benannt. Alle Gesichter von „Ada“ werden „Grace geändert“ zugeordnet. Der Name „Grace geändert“ bleibt erhalten.").assertIsDisplayed()
        compose.onNodeWithText("Trotzdem zusammenführen").performClick();idle(vm)
        assertEquals(1,api.commits);assertEquals("Grace geändert",api.people.getValue(4).name)
    }
    @Test fun namedMergeLostResponseResolvesOnlyConfirmedAction() = screen(setup=::named) {vm,api,_ ->
        api.loseResponse=true
        compose.onNodeWithText("Zusammenführen").performClick()
        compose.onNodeWithText("Trotzdem zusammenführen").performClick();idle(vm)
        assertTrue(vm.state.value.unresolved);assertEquals(1,api.commits)
        compose.onNodeWithText("Zusammenführen").assertIsNotEnabled()
        compose.onNodeWithText("Offene Aktion prüfen").performClick();idle(vm)
        assertFalse(vm.state.value.unresolved);assertEquals(1,api.commits)
        assertEquals(2L,vm.state.value.mergeSuggestion!!.id)
    }
    @Test fun unnamedMergeDoesNotAskForAdditionalConfirmation() = screen(setup=::unnamed) {vm,api,_ ->
        compose.onNodeWithText("Zusammenführen").performClick();idle(vm)
        compose.onNodeWithText("Benannte Gruppen zusammenführen?").assertDoesNotExist()
        assertEquals(1,api.commits);assertEquals("accept_merge",api.receipts.values.single().action)
    }
    @Test fun individualActionsKeepBothSidesUntilNext() = screen(2f,setup=::unnamed) {vm,api,_ ->
        val before=api.people.getValue(4)
        compose.onNodeWithContentDescription("Erste Gruppe ignorieren").performClick();idle(vm)
        assertFalse(api.people.containsKey(3));assertEquals(before,api.people[4])
        assertEquals(1L,vm.state.value.mergeSuggestion!!.id)
        compose.onNodeWithText("Weiter").assertIsDisplayed()
        compose.onNodeWithText("Zusammenführen").assertDoesNotExist()
        compose.onNodeWithText("Getrennt lassen").assertDoesNotExist()
        compose.onNodeWithContentDescription("Zweite Gruppe benennen/zuordnen").performClick()
        compose.onNodeWithText("Name",substring=false).performTextInput("Ada")
        compose.onNodeWithText("Speichern").performClick();idle(vm)
        assertEquals("Ada",api.people.getValue(4).name)
        assertEquals(setOf(3L,4L),vm.state.value.mergeSideResults.keys)
        compose.runOnUiThread {vm.decideMerge(true);vm.decideMerge(false)};idle(vm)
        assertEquals(2,api.commits)
        compose.onNodeWithText("Weiter").performClick();idle(vm)
        assertEquals(2L,vm.state.value.mergeSuggestion!!.id)
        assertTrue(vm.state.value.mergeSideResults.isEmpty())
        assertEquals(setOf("ignore","name"),api.receipts.values.map {it.action}.toSet())
    }
    @Test fun secondSideSearchAssignsOnlySecondAndCancelDoesNothing() = screen(setup=::unnamed) {vm,api,_ ->
        val before=api.people.getValue(3)
        compose.onNodeWithContentDescription("Zweite Gruppe benennen/zuordnen").performClick()
        compose.onNodeWithText("Abbrechen").performClick()
        assertEquals(0,api.commits);assertTrue(vm.state.value.mergeSideResults.isEmpty())
        api.matches=listOf(FaceMatch(6,"Person 6",1,60))
        compose.onNodeWithContentDescription("Zweite Gruppe benennen/zuordnen").performClick()
        compose.onNodeWithContentDescription("Ähnliche benannte Personen suchen").performClick()
        compose.waitUntil(5000) {vm.state.value.faceMatches.isNotEmpty()}
        assertEquals(listOf(40L),api.matchedFaces)
        compose.onNodeWithText("1 Gesicht · #6").performScrollTo().performClick();idle(vm)
        assertEquals(before,api.people[3]);assertFalse(api.people.containsKey(4))
        assertEquals(2L,api.people.getValue(6).count)
        assertEquals(setOf(4L),vm.state.value.mergeSideResults.keys)
        compose.onNodeWithContentDescription("Erste Gruppe ignorieren").assertIsEnabled()
        assertEquals(4L,api.receipts.values.single().source)
    }
    @Test fun individualLostResponseResolvesWithoutAdvancingOrDoubleWrite() = screen(setup=::unnamed) {vm,api,_ ->
        api.loseResponse=true
        compose.onNodeWithContentDescription("Zweite Gruppe ignorieren").performClick();idle(vm)
        assertTrue(vm.state.value.unresolved);assertEquals(1,api.commits)
        compose.onNodeWithContentDescription("Erste Gruppe ignorieren").assertIsNotEnabled()
        compose.onNodeWithText("Offene Aktion prüfen").performClick();idle(vm)
        assertFalse(vm.state.value.unresolved);assertEquals(1,api.commits)
        assertEquals(1L,vm.state.value.mergeSuggestion!!.id)
        compose.onNodeWithContentDescription("Erste Gruppe ignorieren").assertIsEnabled()
        compose.onNodeWithText("Weiter").assertIsDisplayed()
    }
    @Test fun nextSkipsRecreatedReversePairWithoutRejectingIt() = screen(setup=::unnamed) {vm,api,_ ->
        compose.onNodeWithContentDescription("Erste Gruppe benennen/zuordnen").performClick()
        compose.onNodeWithText("Name",substring=false).performTextInput("Ada")
        compose.onNodeWithText("Speichern").performClick();idle(vm)
        val regenerated=MergeSuggestion(99,api.people.getValue(4),api.people.getValue(3),.52)
        api.mergePairs.add(0,regenerated)
        compose.onNodeWithText("Weiter").performClick();idle(vm)
        assertEquals(2L,vm.state.value.mergeSuggestion!!.id)
        assertTrue(regenerated in api.mergePairs);assertEquals(1,api.commits)
    }
    @Test fun namedSidesAndOlderServersHaveNoIndividualButtons() = screen {vm,api,_ ->
        compose.onNodeWithContentDescription("Erste Gruppe ignorieren").assertIsDisplayed()
        compose.onNodeWithContentDescription("Zweite Gruppe ignorieren").assertDoesNotExist()
        compose.onNodeWithContentDescription("Zweite Gruppe benennen/zuordnen").assertDoesNotExist()
        compose.runOnUiThread {vm.ignoreMergeSide(4);vm.startMergeSideNaming(4)};idle(vm)
        assertEquals(0,api.commits);assertFalse(vm.state.value.naming)
        compose.onNodeWithText("Zurück").performClick();idle(vm)
        api.supportsMergeSideActions=false
        compose.onNodeWithText("Menü").performClick();compose.onNodeWithText("Ähnliche Gruppen").performClick();idle(vm)
        compose.onNodeWithContentDescription("Erste Gruppe ignorieren").assertDoesNotExist()
    }
    @Test fun faceSearchUsesMergeWitnessAndAssignsBothGroups() = screen(setup=::unnamed) {vm,api,_ ->
        api.matches=listOf(FaceMatch(6,"Person 6",1,60))
        compose.onNodeWithContentDescription("Zusammenführen und benennen/zuordnen").performClick()
        compose.onNodeWithContentDescription("Ähnliche benannte Personen suchen").performClick()
        compose.waitUntil(5000) {vm.state.value.faceMatches.isNotEmpty()}
        assertEquals(listOf(30L),api.matchedFaces)
        assertEquals(0,api.commits)
        compose.onNodeWithText("1 Gesicht · #6").performScrollTo().performClick();idle(vm)
        assertEquals(1,api.commits)
        assertEquals("name_merge",api.receipts.values.single().action)
        assertEquals(3L,api.people.getValue(6).count)
        assertFalse(api.people.containsKey(3));assertFalse(api.people.containsKey(4))
    }
    @Test fun pencilCancelAndNamePreserveQueue() = screen(2f,setup=::unnamed) {vm,api,db ->
        val pencil=compose.onNodeWithContentDescription("Zusammenführen und benennen/zuordnen")
        pencil.assertIsDisplayed().performClick()
        compose.onNodeWithText("Abbrechen").performClick()
        assertEquals(0,api.commits)
        pencil.performClick()
        compose.onNodeWithText("Name",substring=false).performTextInput("Ada")
        compose.onNodeWithText("Speichern").performClick();idle(vm)
        assertEquals("Ada",api.people.getValue(4).name)
        assertEquals(2L,runBlocking {db.dao().statistics(api.session.scope,0).first().single {it.action=="name"}.groups})
        assertEquals(2L,api.people.getValue(4).count)
        assertFalse(vm.state.value.naming)
        compose.onNodeWithText("Person 6").assertIsDisplayed()
        pencil.assertDoesNotExist()
        compose.onNodeWithText("Zurück").performClick();idle(vm)
        assertEquals(1L,vm.state.value.person!!.id)
        assertEquals(4,vm.state.value.person!!.offset)
    }
    @Test fun pencilAssignLostResponseResolvesOnce() = screen(setup=::unnamed) {vm,api,db ->
        compose.onNodeWithContentDescription("Zusammenführen und benennen/zuordnen").performClick()
        api.loseResponse=true
        compose.onNodeWithText("Name",substring=false).performTextInput("Person 6")
        compose.waitUntil(5000) {vm.state.value.suggestions.any {it.id==6L}}
        compose.onNode(hasText("Person 6") and !hasSetTextAction()).performClick();idle(vm)
        assertEquals(1,api.commits)
        assertTrue(vm.state.value.unresolved)
        val pending=runBlocking {db.dao().pending(api.session.scope)}!!
        val body=org.json.JSONObject(pending.body)
        assertEquals("name_merge",body.getString("action"))
        assertEquals(6L,body.getLong("assign_id"))
        assertEquals(4L,body.getLong("target_id"))
        compose.runOnUiThread {vm.retry()};idle(vm)
        assertEquals(1,api.commits)
        assertEquals(3L,api.people.getValue(6).count)
        assertEquals(2L,runBlocking {db.dao().statistics(api.session.scope,0).first().single {it.action=="assign"}.groups})
        assertFalse(vm.state.value.naming)
        assertFalse(vm.state.value.unresolved)
    }
    @Test fun olderServerOmitsPencil() = screen(setup={unnamed(it);it.supportsMergeNaming=false}) {_,_,_ ->
        compose.onNodeWithContentDescription("Zusammenführen und benennen/zuordnen").assertDoesNotExist()
    }

    @Test fun oneDecisionAtATimeButtonsVisibleWithLargeFontAndQueuePreserved() = screen(2f) {vm,api,_ ->
        compose.onNodeWithTag("merge-similarity").assertIsDisplayed().assertTextEquals("Ähnlichkeit: 0,52")
        compose.onNodeWithText("Person 4").assertIsDisplayed()
        compose.onNodeWithText("Person 6").assertDoesNotExist()
        compose.onNodeWithTag("face-30").assertIsDisplayed()
        compose.onNodeWithTag("face-40").assertIsDisplayed()
        compose.onNodeWithText("Zusammenführen").assertIsDisplayed().performClick();idle(vm)
        compose.onNodeWithText("Person 4").assertDoesNotExist()
        compose.onNodeWithTag("merge-similarity").assertIsDisplayed().assertTextEquals("Ähnlichkeit: 0,68")
        compose.onNodeWithText("Person 6").assertIsDisplayed()
        compose.onNodeWithText("Getrennt lassen").assertIsDisplayed().performClick();idle(vm)
        compose.onNodeWithText("Aktuell keine ähnlichen Gruppen.").assertIsDisplayed()
        compose.onNodeWithTag("merge-similarity").assertDoesNotExist()
        compose.onNodeWithText("Zusammenführen").assertIsNotEnabled()
        compose.onNodeWithText("Getrennt lassen").assertIsNotEnabled()
        assertEquals(2,api.commits)
        assertEquals(2L,api.people.getValue(4).count)
        assertTrue(api.people.containsKey(5))
        compose.onNodeWithText("Zurück").performClick();idle(vm)
        assertEquals(1L,vm.state.value.person!!.id);assertEquals(4,vm.state.value.person!!.offset)
        assertEquals(listOf(14L),vm.state.value.person!!.faces)
    }

    @Test fun bothPortraitsSupportHoldDragCancelAndAccessiblePreviewWithoutDecisions() = screen {_,api,_ ->
        for(faceId in listOf(30,40)) {
            val face=compose.onNodeWithTag("face-$faceId")
            face.performTouchInput {down(center)}
            compose.mainClock.advanceTimeBy(800)
            compose.onNodeWithTag("original-photo-path").assertIsDisplayed().assertTextEquals("Fotos / Urlaub / Bild${faceId/10}.jpg")
            compose.onNodeWithText("Zusammenführen").assertIsNotEnabled()
            face.performTouchInput {moveBy(Offset(0f,100f));moveBy(Offset(0f,-100f));up()}
            compose.onNodeWithTag("original-photo-path").assertDoesNotExist()
            face.performTouchInput {down(center)}
            compose.mainClock.advanceTimeBy(800)
            face.performTouchInput {cancel()}
            compose.onNodeWithTag("original-photo-path").assertDoesNotExist()
            val action=face.fetchSemanticsNode().config[SemanticsActions.CustomActions].single {it.label=="Originalfoto anzeigen"}
            compose.runOnIdle {assertTrue(action.action())}
            compose.onNodeWithText("Vorschau schließen").performClick()
        }
        assertEquals(0,api.commits)
        compose.onAllNodesWithContentDescription("Dieses Gesicht einzeln benennen").assertCountEquals(0)
        compose.onAllNodesWithContentDescription("Zuordnung entfernen").assertCountEquals(0)
    }

    @Test fun lostResponseBlocksDoubleDecisionAndRetryOnlyResolvesReceipt() = screen {vm,api,db ->
        api.loseResponse=true;api.actionDelay=200
        compose.runOnUiThread {vm.decideMerge(true);vm.decideMerge(false)};idle(vm)
        assertEquals(1,api.commits);assertTrue(vm.state.value.unresolved)
        compose.onNodeWithText("Getrennt lassen").assertIsNotEnabled()
        assertNotNull(runBlocking {db.dao().pending(api.session.scope)})
        compose.onNodeWithText("Offene Aktion prüfen").performClick();idle(vm)
        assertEquals(1,api.commits);assertFalse(vm.state.value.unresolved)
        compose.onNodeWithText("Person 6").assertIsDisplayed()
        assertNull(runBlocking {db.dao().pending(api.session.scope)})
    }

    @Test fun failedNextLoadRemovesDecidedCardAndRetryDoesNotWriteAgain() = screen {vm,api,_ ->
        api.failNextMerge=true
        compose.onNodeWithText("Getrennt lassen").performClick();idle(vm)
        assertEquals(1,api.commits);assertNull(vm.state.value.mergeSuggestion)
        compose.onNodeWithText("Zusammenführen").assertIsNotEnabled()
        compose.onNodeWithText("Person 4").assertDoesNotExist()
        api.failNextMerge=false
        compose.onNodeWithText("Erneut versuchen").performClick();idle(vm)
        compose.onNodeWithText("Person 6").assertIsDisplayed();assertEquals(1,api.commits)
    }

    @Test fun conflictRequiresFreshDecisionAndOldServerShowsRequirement() = screen {vm,api,_ ->
        api.people[4]=api.people.getValue(4).copy(revision=2,name="Webänderung")
        api.mergePairs[0]=api.mergePairs[0].copy(target=api.people.getValue(4))
        compose.onNodeWithText("Zusammenführen").performClick();idle(vm)
        assertEquals(0,api.commits);assertFalse(vm.state.value.unresolved)
        compose.onNodeWithText("Webänderung").assertIsDisplayed()
        compose.onNodeWithText("Getrennt lassen").performClick();idle(vm)
        assertEquals(1,api.commits)
        compose.onNodeWithText("Zurück").performClick();idle(vm)
        api.supportsMerges=false
        compose.onNodeWithText("Menü").performClick()
        compose.onNodeWithText("Ähnliche Gruppen").performClick();idle(vm)
        compose.onNodeWithText("Ähnliche Gruppen benötigen BearStack 0.49.0.").assertIsDisplayed()
        compose.onNodeWithText("Zusammenführen").assertIsNotEnabled()
        compose.onNodeWithText("Zurück").performClick();idle(vm)
        assertEquals(4,vm.state.value.person!!.offset)
    }
}
