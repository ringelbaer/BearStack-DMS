package de.bearstack.people

import android.app.Application
import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.createComposeRule
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

class ManualMergeScreenTest {
    @get:Rule val compose=createComposeRule()
    @Test fun selectionFilterHoldPreviewEndlessScrollAndNamedMerge() {
        val app=InstrumentationRegistry.getInstrumentation().targetContext.applicationContext as Application
        val db=Room.inMemoryDatabaseBuilder(app,LabelingDatabase::class.java).build()
        val api=FakeService().apply {
            upper=80
            people.clear()
            for(id in 1L..80L) people[id]=Person(id,if(id%2==0L) "Name $id" else "",1,1,id*10,listOf(id*10),facePaths=mapOf(id*10 to "Fotos / $id.jpg"))
        }
        val store=ViewModelStore();lateinit var vm:PeopleViewModel
        compose.runOnUiThread {vm=PeopleViewModel(app,db,api,api.session);store.put("test",vm)}
        try {
            compose.setGermanContent {PeopleApp(vm)}
            compose.waitUntil(10_000){!vm.state.value.busy && vm.state.value.person!=null}
            compose.onNodeWithText("Menü").performClick()
            compose.onNodeWithText("Gruppen manuell kombinieren").performClick()
            compose.waitUntil(10_000){vm.manualMerges?.state?.value?.pages?.isNotEmpty()==true}
            val controller=vm.manualMerges!!
            assertTrue(controller.state.value.pages.values.flatten().all {it.name.isEmpty()})
            val first=compose.onNodeWithTag("face-10",useUnmergedTree=true)
            first.performTouchInput {click()}
            assertEquals(1,controller.state.value.selected.size)
            compose.onNodeWithText("Kombinieren",substring=false).assertDoesNotExist()
            first.performTouchInput {down(center)}
            compose.mainClock.advanceTimeBy(300)
            compose.onNodeWithTag("manual-merge-preview").assertIsDisplayed()
            first.performTouchInput {up()}
            compose.onNodeWithTag("manual-merge-preview").assertDoesNotExist()
            assertEquals(1,controller.state.value.selected.size)
            compose.onNodeWithContentDescription("Auch benannte Gruppen anzeigen").performClick()
            compose.waitUntil(10_000){!controller.state.value.loading && controller.state.value.includeNamed}
            assertTrue(controller.state.value.selected.isEmpty())
            assertTrue(controller.state.value.pages.values.flatten().any {it.name.isNotEmpty()})
            repeat(3) {
                val last=controller.state.value.count-1
                compose.onNodeWithTag("manual-merge-grid").performScrollToIndex(last)
                compose.waitUntil(10_000){controller.state.value.count>last+1 || !controller.state.value.hasNext}
            }
            assertTrue(controller.state.value.count>40)
            assertTrue(controller.state.value.pages.size<=3)
            compose.onNodeWithTag("manual-merge-grid").performScrollToIndex(0)
            compose.waitUntil(10_000){0 in controller.state.value.pages}
            compose.onNodeWithTag("face-10",useUnmergedTree=true).performTouchInput {click()}
            compose.onNodeWithTag("face-20",useUnmergedTree=true).performTouchInput {click()}
            compose.onNodeWithText("Name bleibt: Name 2").assertIsDisplayed()
            compose.onNodeWithText("Kombinieren").assertIsEnabled()
            compose.onNodeWithText("Kombinieren und benennen").performClick()
            compose.onNodeWithText("Name",substring=false).performTextReplacement("Anna")
            compose.onAllNodesWithText("Kombinieren und benennen").filter(hasAnyAncestor(isDialog()) and hasClickAction()).onFirst().performClick()
            compose.waitUntil(10_000){api.commits==1 && !vm.state.value.busy}
            assertEquals("Anna",api.people[1]!!.name);assertEquals(2L,api.people[1]!!.count)
            assertFalse(api.people.containsKey(2));assertTrue(controller.state.value.selected.isEmpty())
        } finally {compose.runOnUiThread {store.clear()}}
    }
}
