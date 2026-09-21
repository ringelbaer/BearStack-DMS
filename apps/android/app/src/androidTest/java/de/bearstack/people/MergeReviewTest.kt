package de.bearstack.people

import android.app.Application
import android.graphics.Bitmap
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.size
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.asAndroidBitmap
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.unit.Density
import androidx.compose.ui.unit.DpSize
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.height
import androidx.compose.ui.unit.width
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
import java.io.File

class MergeReviewTest {
    @get:Rule val compose=createComposeRule()
    private fun idle(vm: PeopleViewModel) = compose.waitUntil(10_000) {vm.state.value.connected && !vm.state.value.busy}
    private fun screen(scale: Float=1f, setup: (FakeService)->Unit = {}, viewport: DpSize?=null,
        test: (PeopleViewModel,FakeService,LabelingDatabase)->Unit) {
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
                CompositionLocalProvider(LocalDensity provides Density(density.density,scale)) {
                    Box(if(viewport==null) Modifier else Modifier.size(viewport)) {PeopleApp(vm)}
                }
            }
            idle(vm)
            compose.runOnUiThread {vm.page(1)};idle(vm)
            compose.onNodeWithContentDescription("Weitere Optionen").performClick()
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
    private fun alignedPortraits() {
        val first=compose.onNodeWithTag("face-30").assertIsDisplayed().getUnclippedBoundsInRoot()
        val second=compose.onNodeWithTag("face-40").assertIsDisplayed().getUnclippedBoundsInRoot()
        assertEquals(first.top.value,second.top.value,1f)
        assertEquals(first.bottom.value,second.bottom.value,1f)
        assertEquals(first.width.value,second.width.value,1f)
        assertEquals(first.width.value,first.height.value,1f)
        assertTrue(first.right<second.left)
    }
    private fun saveLayout(name: String) {
        val app=InstrumentationRegistry.getInstrumentation().targetContext
        val bitmap=compose.onRoot().captureToImage().asAndroidBitmap()
        File(app.cacheDir,"merge-layout-$name.png").outputStream().use {bitmap.compress(Bitmap.CompressFormat.PNG,100,it)}
    }

    private fun inlineMatches(api: FakeService) {
        unnamed(api)
        api.matches=listOf(FaceMatch(6,"Person 6",1,60))
    }
    private fun matchesReady(vm: PeopleViewModel) = compose.waitUntil(5000) {
        vm.mergeFaceSearch?.state?.value?.let {it.complete || it.error!=null} == true
    }
    @Test fun folderHeadingsUseEachComparisonPhotosContainingFolder() = screen(setup={api ->
        val pair=api.mergePairs[0]
        api.mergePairs[0]=pair.copy(
            source=pair.source.copy(facePaths=mapOf(30L to "Fotos / Reisen / Sommerurlaub / Bild.jpg")),
            target=pair.target.copy(facePaths=mapOf(40L to "Fotos / Familie / Geburtstag / Bild.jpg")))
    }) {_,_,_ ->
        compose.onNodeWithTag("merge-folder-0").assertTextEquals("Sommerurlaub")
        compose.onNodeWithTag("merge-folder-1").assertTextEquals("Geburtstag")
        compose.onNodeWithText("Erste Gruppe").assertDoesNotExist()
        compose.onNodeWithText("Zweite Gruppe").assertDoesNotExist()
        alignedPortraits()
    }

    @Test fun rootFolderAndMissingPathHaveReadableHeadings() = screen(setup={api ->
        val pair=api.mergePairs[0]
        api.mergePairs[0]=pair.copy(
            source=pair.source.copy(facePaths=mapOf(30L to "Fotos / Bild.jpg")),
            target=pair.target.copy(facePaths=emptyMap()))
    }) {_,_,_ ->
        compose.onNodeWithTag("merge-folder-0").assertTextEquals("Fotos")
        compose.onNodeWithTag("merge-folder-1").assertTextEquals("Zweite Gruppe")
    }

    @Test fun fallbackMatchesAssignBothGroupsWithOneAtomicAction() = screen(setup={api ->
        unnamed(api)
        api.matchesByFace=mapOf(40L to listOf(FaceMatch(6,"Person 6",1,60)))
    }) {vm,api,_ ->
        matchesReady(vm)
        assertEquals(listOf(30L,40L),api.matchedFaces)
        compose.onNodeWithTag("merge-match-6").performScrollTo().performClick();idle(vm)
        assertEquals(1,api.commits)
        assertFalse(api.people.containsKey(3));assertFalse(api.people.containsKey(4))
        assertEquals(3L,api.people.getValue(6).count)
    }

    @Test fun inlineSearchUsesOnlyFirstGroupAndAssignsBothWithOneAtomicAction() = screen(setup=::inlineMatches) {vm,api,db ->
        matchesReady(vm)
        assertEquals(listOf(30L),api.matchedFaces)
        assertFalse(vm.state.value.busy);assertEquals(0,api.commits)
        val panel=compose.onNodeWithTag("merge-matches").getUnclippedBoundsInRoot()
        for(side in listOf("Erste","Zweite")) {
            val action=compose.onNodeWithContentDescription("$side Gruppe benennen/zuordnen").getUnclippedBoundsInRoot()
            assertTrue(panel.top>=action.bottom)
        }
        compose.onNodeWithText("Benennungsvorschläge für beide Gruppen").assertIsDisplayed()
        saveLayout("inline-matches")
        // Fetch the current destination revision only when the user selects it.
        api.people[6]=api.people.getValue(6).copy(revision=2)
        compose.onNodeWithContentDescription("Beide Gruppen der Person Person 6 zuordnen").performScrollTo().performClick();idle(vm)
        assertFalse(api.people.containsKey(3));assertFalse(api.people.containsKey(4))
        assertEquals(3L,api.people.getValue(6).count)
        assertEquals(1,api.commits)
        assertEquals("name_merge",api.receipts.values.single().action)
        assertEquals(2L,vm.state.value.mergeSuggestion!!.id)
        compose.onAllNodes(isDialog()).assertCountEquals(0)
        assertEquals(2L,runBlocking {db.dao().statistics(api.session.scope,0).first().single {it.action=="assign"}.groups})
        assertEquals(listOf(30L),api.matchedFaces)
        assertNull(vm.mergeFaceSearch)
    }
    @Test fun inlineStreamingMatchAssignsBothBeforeCompletionAndCancelsSearch() = screen(setup={api ->
        unnamed(api)
        api.matchUpdates=listOf(listOf(FaceMatch(6,"Person 6",1,60)))
        api.matchFinish=kotlinx.coroutines.CompletableDeferred()
    }) {vm,api,_ ->
        compose.waitUntil(5000) {vm.mergeFaceSearch?.state?.value?.matches?.isNotEmpty()==true}
        assertTrue(vm.mergeFaceSearch!!.state.value!!.loading)
        compose.onNodeWithTag("merge-match-6").performScrollTo().performClick();idle(vm)
        assertEquals(1,api.commits);assertEquals(3L,api.people.getValue(6).count)
        assertFalse(api.people.containsKey(3));assertFalse(api.people.containsKey(4))
        assertEquals(1,api.cancelledMatches)
        api.matchFinish!!.complete(Unit)
        assertNull(vm.mergeFaceSearch);assertEquals(2L,vm.state.value.mergeSuggestion!!.id)
        assertEquals(listOf(30L),api.matchedFaces)
    }
    @Test fun inlineRenamedTargetRequiresFreshDecisionAndDeletedTargetDoesNotWrite() = screen(setup=::inlineMatches) {vm,api,db ->
        matchesReady(vm)
        api.people[6]=api.people.getValue(6).copy(name="Neuer Name",revision=2)
        api.matches=listOf(FaceMatch(6,"Neuer Name",1,60))
        compose.onNodeWithTag("merge-match-6").performScrollTo().performClick();idle(vm);matchesReady(vm)
        assertEquals(0,api.commits);assertNotNull(vm.state.value.error)
        assertNull(runBlocking {db.dao().pending(api.session.scope)})
        assertEquals("Neuer Name",vm.mergeFaceSearch!!.state.value!!.matches.single().name)
        api.people.remove(6);api.matches=emptyList()
        compose.onNodeWithTag("merge-match-6").performScrollTo().performClick();idle(vm);matchesReady(vm)
        assertEquals(0,api.commits);assertTrue(vm.mergeFaceSearch!!.state.value!!.matches.isEmpty())
        assertTrue(api.people.containsKey(3));assertTrue(api.people.containsKey(4))
    }
    @Test fun inlineChangedSecondGroupRejectsTheEntireAssignmentAndStopsAutomaticSearch() = screen(setup=::inlineMatches) {vm,api,db ->
        matchesReady(vm)
        api.people[4]=api.people.getValue(4).copy(name="Inzwischen benannt",revision=2)
        api.mergePairs[0]=api.mergePairs[0].copy(target=api.people.getValue(4))
        compose.onNodeWithTag("merge-match-6").performScrollTo().performClick();idle(vm)
        assertEquals(0,api.commits);assertEquals(1L,api.people.getValue(6).count)
        assertTrue(api.people.containsKey(3));assertTrue(api.people.containsKey(4))
        assertNotNull(vm.state.value.error);assertFalse(vm.state.value.unresolved)
        assertNull(runBlocking {db.dao().pending(api.session.scope)})
        assertNull(vm.mergeFaceSearch);assertEquals(listOf(30L),api.matchedFaces)
        compose.onNodeWithTag("merge-matches").assertDoesNotExist()
    }
    @Test fun inlineLostResponseBlocksDoubleAssignmentAndRecoversBothGroups() = screen(setup=::inlineMatches) {vm,api,db ->
        matchesReady(vm)
        val match=vm.mergeFaceSearch!!.state.value!!.matches.single()
        api.loseResponse=true
        compose.onNodeWithTag("merge-match-6").performScrollTo().performClick();idle(vm)
        assertTrue(vm.state.value.unresolved);assertEquals(1,api.commits)
        compose.onNodeWithTag("merge-match-6").assertIsNotEnabled()
        compose.runOnUiThread {vm.assignMergeFaceMatch(match);vm.assignMergeFaceMatch(match)};idle(vm)
        assertEquals(1,api.commits)
        val body=org.json.JSONObject(runBlocking {db.dao().pending(api.session.scope)}!!.body)
        assertEquals("name_merge",body.getString("action"))
        assertEquals(4L,body.getLong("target_id"));assertEquals(6L,body.getLong("assign_id"))
        compose.onNodeWithText("Offene Aktion prüfen").performScrollTo().performClick();idle(vm)
        assertFalse(vm.state.value.unresolved);assertEquals(1,api.commits)
        assertEquals(3L,api.people.getValue(6).count);assertEquals(2L,vm.state.value.mergeSuggestion!!.id)
        assertNull(vm.mergeFaceSearch)
    }
    @Test fun inlineFailureAllowsManualDecisionsAndExplicitRetryUsesOnlyFirstGroup() = screen(setup={api ->
        inlineMatches(api);api.matchFailure=true
    }) {vm,api,_ ->
        matchesReady(vm)
        assertNull(vm.state.value.error);assertFalse(vm.state.value.busy)
        assertEquals(listOf(30L),api.matchedFaces)
        compose.onNodeWithText("Zusammenführen").assertIsEnabled()
        api.matchFailure=false
        compose.onNode(hasText("Erneut versuchen") and hasAnyAncestor(hasTestTag("merge-matches")))
            .performScrollTo().performClick()
        compose.waitUntil(5000) {vm.mergeFaceSearch!!.state.value!!.complete}
        assertEquals(listOf(30L,30L),api.matchedFaces)
        compose.onNodeWithTag("merge-match-6").assertIsEnabled()
    }
    @Test fun inlineLongRankingScrollsAtLargeFontWithoutMovingTheDecisionButtons() = screen(1.5f,setup={api ->
        unnamed(api)
        api.matches=(100L..119L).map {FaceMatch(it,"Ein langer Personenname $it",12,it*10)}
    },viewport=DpSize(320.dp,640.dp)) {vm,api,_ ->
        matchesReady(vm)
        val merge=compose.onNodeWithText("Zusammenführen").assertIsDisplayed().getUnclippedBoundsInRoot()
        val reject=compose.onNodeWithText("Getrennt lassen").assertIsDisplayed().getUnclippedBoundsInRoot()
        compose.onNodeWithTag("merge-match-119").performScrollTo().assertIsDisplayed()
        assertEquals(merge,compose.onNodeWithText("Zusammenführen").assertIsDisplayed().getUnclippedBoundsInRoot())
        assertEquals(reject,compose.onNodeWithText("Getrennt lassen").assertIsDisplayed().getUnclippedBoundsInRoot())
        assertEquals(listOf(30L),api.matchedFaces);assertEquals(0,api.commits)
    }
    @Test fun inlineSuggestionsDisappearAfterAnIndividualActionOnSecondGroup() = screen(setup=::inlineMatches) {vm,api,_ ->
        matchesReady(vm)
        val match=vm.mergeFaceSearch!!.state.value!!.matches.single()
        compose.onNodeWithContentDescription("Zweite Gruppe ignorieren").performScrollTo().performClick();idle(vm)
        compose.onNodeWithTag("merge-matches").assertDoesNotExist()
        compose.runOnUiThread {vm.assignMergeFaceMatch(match)};idle(vm)
        assertEquals(1,api.commits);assertEquals("ignore",api.receipts.values.single().action)
        assertTrue(api.people.containsKey(3));assertEquals(1L,api.people.getValue(6).count)
        assertNull(vm.mergeFaceSearch);assertEquals(listOf(30L),api.matchedFaces)
    }
    @Test fun namedFirstGroupDoesNotStartAutomaticSearch() = screen(setup={api ->
        unnamed(api)
        api.people[3]=api.people.getValue(3).copy(name="Ada")
        api.mergePairs[0]=api.mergePairs[0].copy(source=api.people.getValue(3))
    }) {vm,api,_ ->
        compose.onNodeWithTag("merge-matches").assertDoesNotExist()
        assertNull(vm.mergeFaceSearch);assertTrue(api.matchedFaces.isEmpty())
    }
    @Test fun inlineSuggestionsNeedSharedNamingCapabilityOnly() = screen(setup={api ->
        inlineMatches(api);api.supportsMergeSideActions=false
    }) {vm,api,_ ->
        matchesReady(vm)
        compose.onNodeWithTag("merge-match-6").performScrollTo().performClick();idle(vm)
        assertEquals("name_merge",api.receipts.values.single().action)
        assertEquals(3L,api.people.getValue(6).count);assertEquals(listOf(30L),api.matchedFaces)
    }
    @Test fun mixedGroupsHaveAlignedPortraitsAndCompactActions() = screen(viewport=DpSize(360.dp,760.dp)) {vm,api,_ ->
        assertNull(vm.mergeFaceSearch);assertTrue(api.matchedFaces.isEmpty())
        compose.onNodeWithTag("merge-matches").assertDoesNotExist()
        alignedPortraits()
        val portrait=compose.onNodeWithTag("face-30").getUnclippedBoundsInRoot()
        val label=compose.onNodeWithTag("merge-folder-0").getUnclippedBoundsInRoot()
        assertTrue("Portrait follows its label without a large gap",portrait.top-label.bottom<=16.dp)
        val ignore=compose.onNodeWithContentDescription("Erste Gruppe ignorieren").assertIsDisplayed().getUnclippedBoundsInRoot()
        val pencil=compose.onNodeWithContentDescription("Erste Gruppe benennen/zuordnen").assertIsDisplayed().getUnclippedBoundsInRoot()
        assertEquals(ignore.top.value,pencil.top.value,1f)
        compose.onNodeWithContentDescription("Erste Gruppe ignorieren").assertHeightIsAtLeast(48.dp)
        compose.onNodeWithContentDescription("Erste Gruppe benennen/zuordnen").assertHeightIsAtLeast(48.dp).assertWidthIsAtLeast(48.dp)
        assertTrue(ignore.right<=pencil.left)
        assertTrue(ignore.top>portrait.bottom)
        compose.onNodeWithText("Zusammenführen").assertIsDisplayed()
        compose.onNodeWithText("Getrennt lassen").assertIsDisplayed()
        saveLayout("mixed")
        compose.onNodeWithContentDescription("Erste Gruppe ignorieren").performClick();idle(vm)
        assertEquals(2L,vm.state.value.mergeSuggestion!!.id)
        compose.onNodeWithTag("face-50").assertIsDisplayed()
        compose.onNodeWithText("Weiter").assertDoesNotExist()
    }
    @Test fun wrappedNameKeepsPortraitsAndCountsAligned() = screen(setup={api ->
        api.people[4]=api.people.getValue(4).copy(name="Alexandra Musterfrau mit langem Namen",count=107)
        api.mergePairs[0]=api.mergePairs[0].copy(target=api.people.getValue(4))
    },viewport=DpSize(320.dp,640.dp)) {_,_,_ ->
        alignedPortraits()
        val first=compose.onNodeWithText("1 Gesicht").getUnclippedBoundsInRoot()
        val second=compose.onNodeWithText("107 Gesichter").getUnclippedBoundsInRoot()
        assertEquals(first.top.value,second.top.value,1f)
        compose.onNodeWithContentDescription("Erste Gruppe ignorieren").assertIsDisplayed()
        compose.onNodeWithContentDescription("Erste Gruppe benennen/zuordnen").assertIsDisplayed()
        saveLayout("long-name")
    }
    @Test fun smallScreenLargeFontScrollsDetailsAndKeepsDecisionsVisible() = screen(2f,setup=::unnamed,
        viewport=DpSize(320.dp,480.dp)) {vm,api,_ ->
        val merge=compose.onNodeWithText("Zusammenführen")
        val reject=compose.onNodeWithText("Getrennt lassen")
        val mergeBounds=merge.assertIsDisplayed().getUnclippedBoundsInRoot()
        val rejectBounds=reject.assertIsDisplayed().getUnclippedBoundsInRoot()
        compose.onNodeWithContentDescription("Zweite Gruppe benennen/zuordnen").performScrollTo().assertIsDisplayed().performClick()
        compose.onNodeWithText("Abbrechen").performClick()
        compose.onNodeWithContentDescription("Erste Gruppe ignorieren").performScrollTo().assertIsDisplayed()
        assertEquals(mergeBounds,merge.assertIsDisplayed().getUnclippedBoundsInRoot())
        assertEquals(rejectBounds,reject.assertIsDisplayed().getUnclippedBoundsInRoot())
        saveLayout("large-font")
        reject.performClick();idle(vm)
        compose.onNodeWithTag("face-50").assertIsDisplayed()
        assertEquals(1,api.commits)
    }
    @Test fun namedMergeRequiresConfirmationAndCancelDoesNotWrite() = screen(2f,setup=::named) {vm,api,db ->
        assertNull(vm.mergeFaceSearch);assertTrue(api.matchedFaces.isEmpty())
        compose.onNodeWithTag("merge-matches").assertDoesNotExist()
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
    @Test fun namedLinkOpensPersonAndBackLoadsDirectoryWithoutChangingQueue() = screen {vm,api,db ->
        val before=runBlocking {db.dao().state(api.session.scope)}!!
        compose.onNodeWithTag("merge-person-4").performScrollTo().performClick();idle(vm)
        assertTrue(vm.state.value.directory);assertFalse(vm.state.value.mergeReview)
        assertEquals(4L,vm.state.value.selectedPerson!!.id)
        assertEquals(listOf(40L),vm.state.value.selectedPerson!!.faces)
        assertTrue(vm.state.value.batchFaces)
        assertTrue(api.directoryQueries.isEmpty())
        compose.onNodeWithContentDescription("Weitere Optionen").performClick()
        compose.onNodeWithText("Zurück").performClick();idle(vm)
        assertNull(vm.state.value.selectedPerson)
        assertEquals(setOf(4L,6L),vm.state.value.namedPeople.map {it.id}.toSet())
        compose.onNodeWithContentDescription("Weitere Optionen").performClick()
        compose.onNodeWithText("Zurück").performClick();idle(vm)
        assertFalse(vm.state.value.directory)
        val after=runBlocking {db.dao().state(api.session.scope)}!!
        assertEquals(before.current,after.current);assertEquals(before.page,after.page)
        assertEquals(0,api.commits)
    }
    @Test fun personLinkLoadFailureRetriesOnlyDetails() = screen {vm,api,_ ->
        api.failPerson=4
        compose.onNodeWithTag("merge-person-4").performClick();idle(vm)
        assertTrue(vm.state.value.directory);assertNotNull(vm.state.value.error)
        api.failPerson=null
        compose.runOnUiThread {vm.retry()};idle(vm)
        assertEquals(listOf(40L),vm.state.value.selectedPerson!!.faces)
        assertEquals(0,api.commits)
    }
    @Test fun removedPersonLinkFallsBackToFreshDirectory() = screen {vm,api,_ ->
        api.people.remove(4)
        compose.onNodeWithTag("merge-person-4").performClick();idle(vm)
        assertTrue(vm.state.value.directory);assertNull(vm.state.value.selectedPerson)
        assertEquals(listOf(6L),vm.state.value.namedPeople.map {it.id})
        assertNull(vm.state.value.error);assertEquals(0,api.commits)
    }
    @Test fun namedTargetAndIgnoredSourceAdvanceAutomatically() = screen {vm,api,_ ->
        compose.onNodeWithContentDescription("Erste Gruppe ignorieren").performClick();idle(vm)
        assertEquals(2L,vm.state.value.mergeSuggestion!!.id)
        assertEquals(1,api.commits)
    }
    @Test fun namedSourceAndNamedOtherSideAdvanceAutomatically() = screen(setup={api ->
        named(api)
        api.people[4]=api.people.getValue(4).copy(name="")
        api.mergePairs[0]=api.mergePairs[0].copy(target=api.people.getValue(4))
    }) {vm,api,_ ->
        compose.onNodeWithContentDescription("Zweite Gruppe benennen/zuordnen").performClick()
        compose.onNodeWithText("Name",substring=false).performTextInput("Grace")
        compose.onNodeWithText("Speichern").performClick();idle(vm)
        assertEquals("Grace",api.people.getValue(4).name)
        assertEquals(2L,vm.state.value.mergeSuggestion!!.id);assertEquals(1,api.commits)
    }
    @Test fun assignedTargetNameSurvivesLostResponseAndLinkIsBlockedUntilResolved() = screen(setup=::unnamed) {vm,api,_ ->
        api.loseResponse=true
        compose.runOnUiThread {vm.startMergeSideNaming(3);vm.assign(api.people.getValue(6))};idle(vm)
        assertTrue(vm.state.value.unresolved);assertEquals(1L,vm.state.value.mergeSuggestion!!.id)
        compose.runOnUiThread {vm.openMergePerson(3)};idle(vm)
        assertFalse(vm.state.value.directory)
        compose.runOnUiThread {vm.retry()};idle(vm)
        compose.onNodeWithText("Zugeordnet: Person 6").performScrollTo().performClick();idle(vm)
        assertEquals(6L,vm.state.value.selectedPerson!!.id);assertEquals(1,api.commits)
    }
    @Test fun namedSideAndAssignmentAdvanceAfterReceiptWithoutDoubleWrite() = screen {vm,api,_ ->
        api.loseResponse=true
        compose.runOnUiThread {vm.startMergeSideNaming(3);vm.assign(api.people.getValue(4))};idle(vm)
        assertTrue(vm.state.value.unresolved);assertEquals(1L,vm.state.value.mergeSuggestion!!.id)
        compose.runOnUiThread {vm.openMergePerson(4)};idle(vm)
        assertFalse(vm.state.value.directory)
        compose.runOnUiThread {vm.retry()};idle(vm)
        assertEquals(2L,vm.state.value.mergeSuggestion!!.id);assertEquals(1,api.commits)
    }
    @Test fun newlyNamedSideLinksToPersonWhileOtherSideRemainsEditable() = screen(setup=::unnamed) {vm,api,_ ->
        compose.onNodeWithContentDescription("Erste Gruppe benennen/zuordnen").performClick()
        compose.onNodeWithText("Name",substring=false).performTextInput("Ada")
        compose.onNodeWithText("Speichern").performClick();idle(vm)
        compose.onNodeWithContentDescription("Zweite Gruppe ignorieren").assertIsEnabled()
        compose.onNodeWithTag("merge-person-3").performClick();idle(vm)
        assertEquals(3L,vm.state.value.selectedPerson!!.id)
        assertEquals("Ada",vm.state.value.selectedPerson!!.name);assertEquals(1,api.commits)
    }
    @Test fun ignoringBothSidesAutomaticallyLoadsNextPair() = screen(setup=::unnamed) {vm,api,_ ->
        compose.onNodeWithContentDescription("Erste Gruppe ignorieren").performClick();idle(vm)
        assertEquals(1L,vm.state.value.mergeSuggestion!!.id)
        compose.onNodeWithText("Weiter").assertIsDisplayed()
        compose.onNodeWithContentDescription("Zweite Gruppe ignorieren").performClick();idle(vm)
        assertEquals(2L,vm.state.value.mergeSuggestion!!.id)
        assertTrue(vm.state.value.mergeSideResults.isEmpty());assertTrue(vm.state.value.mergeSidePeople.isEmpty())
        assertEquals(2,api.commits);assertEquals(setOf(3L,4L),api.receipts.values.map {it.source}.toSet())
        assertTrue(api.receipts.values.all {it.action=="ignore"})
        compose.onNodeWithText("Weiter").assertDoesNotExist()
        compose.onNodeWithText("Zusammenführen").assertIsEnabled()
    }
    @Test fun secondIgnoreLostResponseAdvancesOnlyAfterReceiptAndNeverWritesTwice() = screen(setup=::unnamed) {vm,api,_ ->
        compose.onNodeWithContentDescription("Zweite Gruppe ignorieren").performClick();idle(vm)
        api.loseResponse=true
        compose.onNodeWithContentDescription("Erste Gruppe ignorieren").performClick();idle(vm)
        assertTrue(vm.state.value.unresolved);assertEquals(1L,vm.state.value.mergeSuggestion!!.id)
        assertEquals(setOf(4L),vm.state.value.mergeSideResults.keys);assertEquals(2,api.commits)
        compose.onNodeWithText("Weiter").assertIsNotEnabled()
        compose.onNodeWithText("Offene Aktion prüfen").performClick();idle(vm)
        assertFalse(vm.state.value.unresolved);assertEquals(2L,vm.state.value.mergeSuggestion!!.id)
        assertEquals(2,api.commits);assertTrue(vm.state.value.mergeSideResults.keys.isEmpty())
    }
    @Test fun automaticNextLoadFailureCanRetryWithoutRepeatingIgnores() = screen(setup=::unnamed) {vm,api,_ ->
        compose.onNodeWithContentDescription("Erste Gruppe ignorieren").performClick();idle(vm)
        api.failNextMerge=true
        compose.onNodeWithContentDescription("Zweite Gruppe ignorieren").performClick();idle(vm)
        assertEquals(2,api.commits);assertNull(vm.state.value.mergeSuggestion)
        assertNotNull(vm.state.value.error);assertFalse(vm.state.value.unresolved)
        assertTrue(vm.state.value.mergeSideResults.keys.isEmpty())
        api.failNextMerge=false
        compose.onNodeWithText("Erneut versuchen").performClick();idle(vm)
        assertEquals(2L,vm.state.value.mergeSuggestion!!.id);assertEquals(2,api.commits)
    }
    @Test fun ignoringLastPairShowsEmptyStateWithoutExtraDecision() = screen(setup=::unnamed) {vm,api,_ ->
        api.mergePairs.removeAt(1)
        compose.onNodeWithContentDescription("Erste Gruppe ignorieren").performClick();idle(vm)
        compose.onNodeWithContentDescription("Zweite Gruppe ignorieren").performClick();idle(vm)
        assertNull(vm.state.value.mergeSuggestion);assertNull(vm.state.value.error)
        assertEquals(2,api.commits);assertTrue(vm.state.value.mergeSideResults.isEmpty())
        compose.onNodeWithText("Weiter").assertDoesNotExist()
    }
    @Test fun individualActionsAdvanceOnceBothSidesAreProcessed() = screen(2f,setup=::unnamed) {vm,api,_ ->
        val before=api.people.getValue(4)
        compose.onNodeWithContentDescription("Erste Gruppe ignorieren").performClick();idle(vm)
        assertFalse(api.people.containsKey(3));assertEquals(before,api.people[4])
        assertNull(vm.mergeFaceSearch)
        compose.onNodeWithTag("merge-matches").assertDoesNotExist()
        assertEquals(1L,vm.state.value.mergeSuggestion!!.id)
        compose.onNodeWithText("Weiter").assertIsDisplayed()
        compose.onNodeWithText("Zusammenführen").assertDoesNotExist()
        compose.onNodeWithText("Getrennt lassen").assertDoesNotExist()
        compose.onNodeWithContentDescription("Zweite Gruppe benennen/zuordnen").performClick()
        compose.onNodeWithText("Name",substring=false).performTextInput("Ada")
        compose.onNodeWithText("Speichern").performClick();idle(vm)
        assertEquals("Ada",api.people.getValue(4).name)
        assertEquals(2,api.commits)
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
        val beforeSearch=api.matchedFaces.size
        compose.onNodeWithContentDescription("Ähnliche benannte Personen suchen").performClick()
        compose.waitUntil(5000) {vm.state.value.faceMatches.isNotEmpty()}
        assertEquals(listOf(40L),api.matchedFaces.drop(beforeSearch))
        compose.onNode(hasText("1 Gesicht · #6") and hasAnyAncestor(isDialog())).performScrollTo().performClick();idle(vm)
        assertEquals(before,api.people[3]);assertFalse(api.people.containsKey(4))
        assertEquals(2L,api.people.getValue(6).count)
        assertEquals(setOf(4L),vm.state.value.mergeSideResults.keys)
        compose.onNodeWithContentDescription("Erste Gruppe ignorieren").assertIsEnabled()
        assertEquals(4L,api.receipts.values.single().source)
        compose.onNodeWithText("Zugeordnet: Person 6").assertIsDisplayed()
        compose.onNodeWithTag("merge-person-4").performScrollTo().performClick();idle(vm)
        assertTrue(vm.state.value.directory);assertFalse(vm.state.value.mergeReview)
        assertEquals(6L,vm.state.value.selectedPerson!!.id)
        assertEquals(setOf(40L,60L),vm.state.value.selectedPerson!!.faces.toSet())
        assertEquals(1,api.commits)
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
        assertEquals("Ada",api.people.getValue(3).name)
        assertEquals(1,api.commits)
        val regenerated=MergeSuggestion(99,api.people.getValue(4),api.people.getValue(3),.52)
        api.mergePairs.add(0,regenerated)
        // The closing keyboard can still move the bottom bar. Invoke its
        // accessible click action to test pair exclusion independently of that animation.
        compose.onNodeWithText("Weiter").assertIsEnabled().performSemanticsAction(SemanticsActions.OnClick) {assertTrue(it())};idle(vm)
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
        compose.onNodeWithContentDescription("Weitere Optionen").performClick();compose.onNodeWithText("Ähnliche Gruppen").performClick();idle(vm)
        compose.onNodeWithContentDescription("Erste Gruppe ignorieren").assertDoesNotExist()
    }
    @Test fun faceSearchUsesMergeWitnessAndAssignsBothGroups() = screen(setup=::unnamed) {vm,api,_ ->
        api.matches=listOf(FaceMatch(6,"Person 6",1,60))
        compose.onNodeWithContentDescription("Zusammenführen und benennen/zuordnen").performClick()
        val beforeSearch=api.matchedFaces.size
        compose.onNodeWithContentDescription("Ähnliche benannte Personen suchen").performClick()
        compose.waitUntil(5000) {vm.state.value.faceMatches.isNotEmpty()}
        assertEquals(listOf(30L),api.matchedFaces.drop(beforeSearch))
        assertEquals(0,api.commits)
        compose.onNode(hasText("1 Gesicht · #6") and hasAnyAncestor(isDialog())).performScrollTo().performClick();idle(vm)
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
    @Test fun olderServerOmitsPencil() = screen(setup={unnamed(it);it.supportsMergeNaming=false}) {vm,api,_ ->
        assertNull(vm.mergeFaceSearch);assertTrue(api.matchedFaces.isEmpty())
        compose.onNodeWithTag("merge-matches").assertDoesNotExist()
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
        compose.onNodeWithContentDescription("Weitere Optionen").performClick()
        compose.onNodeWithText("Ähnliche Gruppen").performClick();idle(vm)
        compose.onNodeWithText("Ähnliche Gruppen benötigen BearStack 0.49.0.").assertIsDisplayed()
        compose.onNodeWithText("Zusammenführen").assertIsNotEnabled()
        compose.onNodeWithText("Zurück").performClick();idle(vm)
        assertEquals(4,vm.state.value.person!!.offset)
    }
}
