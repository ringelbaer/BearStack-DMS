package de.bearstack.people

import android.app.Application
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.unit.Density
import androidx.lifecycle.ViewModelStore
import androidx.room.Room
import androidx.test.platform.app.InstrumentationRegistry
import de.bearstack.people.data.local.LabelingDatabase
import de.bearstack.people.data.remote.LabelingService
import de.bearstack.people.data.remote.Person
import de.bearstack.people.people.PeopleViewModel
import de.bearstack.people.ui.PeopleApp
import kotlinx.coroutines.CompletableDeferred
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test

class LoadingLayoutTest {
    @get:Rule val compose=createComposeRule()

    @Test fun loadingKeepsFacesAndNavigationInPlace() = checkLoadingLayout(1f,false)

    @Test fun loadingKeepsScrolledContentInPlaceWithLargeFont() = checkLoadingLayout(2f,true)

    @Test fun fourFacesDoNotShowPagination() {
        val app=InstrumentationRegistry.getInstrumentation().targetContext.applicationContext as Application
        val db=Room.inMemoryDatabaseBuilder(app,LabelingDatabase::class.java).build()
        val fake=FakeService().apply { people[1]=people.getValue(1).copy(count=4) }
        val store=ViewModelStore()
        lateinit var vm: PeopleViewModel
        compose.runOnUiThread {vm=PeopleViewModel(app,db,fake,fake.session);store.put("test",vm)}
        try {
            compose.setContent {PeopleApp(vm)}
            compose.waitUntil(10_000){!vm.state.value.busy && vm.state.value.person!=null}
            compose.onNodeWithText("Zurück").assertDoesNotExist()
            compose.onNodeWithText("Weiter").assertDoesNotExist()
        } finally {compose.runOnUiThread {store.clear()}}
    }

    @Test fun stickyActionsSeparateNavigationHelpAndNaming() {
        val app=InstrumentationRegistry.getInstrumentation().targetContext.applicationContext as Application
        val db=Room.inMemoryDatabaseBuilder(app,LabelingDatabase::class.java).build()
        val fake=FakeService().apply { people[1]=people.getValue(1).copy(count=8) }
        val store=ViewModelStore()
        lateinit var vm: PeopleViewModel
        compose.runOnUiThread {vm=PeopleViewModel(app,db,fake,fake.session);store.put("test",vm)}
        try {
            compose.setContent {
                val density=LocalDensity.current
                CompositionLocalProvider(LocalDensity provides Density(density.density,2f)) {PeopleApp(vm)}
            }
            compose.waitUntil(10_000){!vm.state.value.busy && vm.state.value.person!=null}
            val bar=compose.onNodeWithTag("labeling-action-bar")
            val bounds=bar.getUnclippedBoundsInRoot()
            val actions=compose.onNodeWithContentDescription("Gruppenaktionen")
            val help=compose.onNodeWithContentDescription("Hilfe zum Benennen")
            val pencil=compose.onNodeWithContentDescription("Person benennen")
            assertTrue(actions.getUnclippedBoundsInRoot().left < help.getUnclippedBoundsInRoot().left)
            assertTrue(help.getUnclippedBoundsInRoot().left < pencil.getUnclippedBoundsInRoot().left)
            compose.onNodeWithText("Weiter").performScrollTo()
            assertEquals(bounds,bar.getUnclippedBoundsInRoot())
            compose.onNodeWithText("Menü").performClick()
            compose.onNodeWithText("Gruppe ignorieren").assertDoesNotExist()
            compose.onNodeWithText("Statistik").performClick()
            bar.assertDoesNotExist()
            compose.onNodeWithText("Menü").performClick()
            compose.onNodeWithText("Zur Bearbeitung").performClick()
            help.performClick()
            compose.onNodeWithText("Stift: Person benennen oder einer vorhandenen Person zuordnen.").assertIsDisplayed()
            compose.onNodeWithText("Schließen").performClick()
            actions.performClick()
            compose.onNodeWithText("Gruppe ignorieren").assertIsEnabled()
            compose.onNodeWithText("Letztes Überspringen zurücknehmen").assertIsNotEnabled()
            compose.onNodeWithText("Gruppe überspringen").performClick()
            compose.waitUntil(10_000){!vm.state.value.busy && vm.state.value.canGoBack}
            actions.performClick()
            compose.onNodeWithText("Letztes Überspringen zurücknehmen").performClick()
            compose.waitUntil(10_000){!vm.state.value.busy && vm.state.value.person?.id==1L}
            pencil.performClick()
            compose.onNodeWithText("Name").assertExists()
            compose.onNodeWithText("Abbrechen").performClick()
        } finally {compose.runOnUiThread {store.clear()}}
    }

    private fun checkLoadingLayout(fontScale: Float, scrollToNavigation: Boolean) {
        val app=InstrumentationRegistry.getInstrumentation().targetContext.applicationContext as Application
        val db=Room.inMemoryDatabaseBuilder(app,LabelingDatabase::class.java).build()
        val fake=FakeService().apply { people[1]=people.getValue(1).copy(count=8) }
        var response: CompletableDeferred<Unit>?=null
        val service=object : LabelingService by fake {
            override suspend fun person(id: Long,offset: Int): Person {
                response?.await()
                return fake.person(id,offset).copy(offset=offset,faces=(10L+offset..13L+offset).toList())
            }
        }
        val store=ViewModelStore()
        lateinit var vm: PeopleViewModel
        compose.runOnUiThread {vm=PeopleViewModel(app,db,service,fake.session);store.put("test",vm)}
        try {
            compose.setContent {
                val density=LocalDensity.current
                CompositionLocalProvider(LocalDensity provides Density(density.density,fontScale)) {PeopleApp(vm)}
            }
            compose.waitUntil(10_000){!vm.state.value.busy && vm.state.value.person!=null}
            if(scrollToNavigation) compose.onNodeWithText("Weiter").performScrollTo()
            val progress=compose.onNode(SemanticsMatcher.keyIsDefined(SemanticsProperties.ProgressBarRangeInfo))
            val content=listOf(compose.onNodeWithTag("face-grid"),
                compose.onNodeWithText("Zurück"),compose.onNodeWithText("Weiter"))
            val bounds=content.map { it.getUnclippedBoundsInRoot() }
            progress.assertDoesNotExist()
            for(delta in listOf(1,-1)) {
                val pending=CompletableDeferred<Unit>()
                compose.runOnIdle {response=pending;vm.page(delta)}
                compose.waitUntil(10_000){vm.state.value.busy}
                progress.assertExists()
                content.forEachIndexed { index,node -> assertEquals("Content moved when loading started",bounds[index],node.getUnclippedBoundsInRoot()) }
                compose.onNodeWithText("Weiter").assertIsNotEnabled()
                compose.onNodeWithText("Zurück").assertIsNotEnabled()
                compose.runOnIdle {pending.complete(Unit)}
                compose.waitUntil(10_000){!vm.state.value.busy}
                progress.assertDoesNotExist()
                content.forEachIndexed { index,node -> assertEquals("Content moved when loading finished",bounds[index],node.getUnclippedBoundsInRoot()) }
                compose.runOnIdle {
                    assertNull(vm.state.value.error)
                    assertEquals(if(delta>0) 4 else 0,vm.state.value.person!!.offset)
                }
            }
        } finally {compose.runOnUiThread {store.clear()}}
    }
}
