package de.bearstack.people

import android.app.Application
import android.content.res.Configuration
import androidx.compose.runtime.*
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.lifecycle.ViewModelStore
import androidx.room.Room
import androidx.test.platform.app.InstrumentationRegistry
import de.bearstack.people.connection.*
import de.bearstack.people.data.local.LabelingDatabase
import de.bearstack.people.data.remote.*
import de.bearstack.people.people.PeopleViewModel
import de.bearstack.people.text.*
import de.bearstack.people.ui.PeopleApp
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import java.io.IOException
import java.util.Locale

class LocalizationTest {
    @get:Rule val compose=createComposeRule()
    private fun idle(vm: PeopleViewModel)=compose.waitUntil(10_000) {vm.state.value.connected && !vm.state.value.busy}
    private fun screen(test: (PeopleViewModel,FakeService,(Locale)->Unit)->Unit) {
        val app=InstrumentationRegistry.getInstrumentation().targetContext.applicationContext as Application
        val db=Room.inMemoryDatabaseBuilder(app,LabelingDatabase::class.java).build()
        val api=FakeService().apply {
            people[2]=people.getValue(2).copy(name="Ada",facePaths=mapOf(20L to "Fotos / Urlaub / Bild.jpg"))
            mergePairs+=MergeSuggestion(1,people.getValue(1).copy(faces=listOf(10)),people.getValue(2))
        }
        val store=ViewModelStore()
        lateinit var vm:PeopleViewModel
        var locale by mutableStateOf(Locale.GERMAN)
        compose.runOnUiThread {vm=PeopleViewModel(app,db,api,api.session);store.put("test",vm)}
        try {
            compose.setContent {
                val context=remember(locale) {app.createConfigurationContext(Configuration(app.resources.configuration).apply {setLocale(locale)})}
                CompositionLocalProvider(LocalContext provides context,LocalConfiguration provides context.resources.configuration) {PeopleApp(vm)}
            }
            idle(vm)
            test(vm,api) {next ->compose.runOnUiThread {locale=next};compose.waitForIdle()}
        } finally {compose.runOnUiThread {store.clear()}}
    }

    @Test fun languageChangeRetranslatesExistingErrorWithoutResettingTheQueue()=screen {vm,api,language ->
        val original=vm.state.value.person
        compose.runOnUiThread {vm.connect("http://user:secret@private.invalid/?token=secret","user","password")}
        idle(vm)
        val error=vm.state.value.error!!
        assertEquals(R.string.error_address,error.resource)
        compose.onNodeWithText("Eine HTTPS-Adresse ohne Zugangsdaten, Abfrage oder Fragment eingeben.").assertIsDisplayed()
        language(Locale.ENGLISH)
        compose.onNodeWithText("Enter an HTTPS address without credentials, a query or a fragment.").assertIsDisplayed()
        compose.onNodeWithContentDescription("Name person").assertIsDisplayed()
        assertEquals(error,vm.state.value.error)
        assertEquals(original,vm.state.value.person)
        assertEquals(0,api.commits)
        compose.onAllNodes(hasText("secret",substring=true)).assertCountEquals(0)
        language(Locale.GERMAN)
        compose.onNodeWithContentDescription("Person benennen").assertIsDisplayed()
        assertEquals(error,vm.state.value.error)
    }

    @Test fun englishNamingDirectoryHelpAndStatisticsUseNativeResources()=screen {vm,api,language ->
        language(Locale.ENGLISH)
        compose.onNodeWithContentDescription("Naming help").performClick()
        compose.onNodeWithText("Pencil: name the person or assign the group to an existing person.").assertIsDisplayed()
        compose.onNodeWithText("Close").performClick()
        compose.onNodeWithContentDescription("Name person").performClick()
        compose.onNode(hasSetTextAction()).performTextInput("Grace")
        compose.onNodeWithText("Save").performClick();idle(vm)
        assertEquals("name",api.receipts.values.single().action)
        compose.onNodeWithText("Pass complete").assertIsDisplayed()
        compose.onNodeWithText("Menu").performClick()
        compose.onNodeWithText("Statistics").performClick()
        compose.waitUntil(10_000) {compose.onAllNodesWithText("Total: 5 faces · 1 group").fetchSemanticsNodes().isNotEmpty()}
        compose.onNodeWithText("Total: 5 faces · 1 group").assertIsDisplayed()
        compose.onNodeWithText("Menu").performClick()
        compose.onNodeWithText("People").performClick();idle(vm)
        compose.onNodeWithText("Search people").assertIsDisplayed()
        compose.onNodeWithText("Ada").performClick();idle(vm)
        compose.onNodeWithText("1 face").assertIsDisplayed()
        compose.onNodeWithText("Photos / Urlaub / Bild.jpg").assertExists()
        compose.onNodeWithText("Rename person").performClick()
        compose.onNode(hasSetTextAction()).performTextReplacement("Ada Lovelace")
        compose.onNodeWithText("Save").performClick();idle(vm)
        assertEquals("Ada Lovelace",api.people.getValue(2).name)
    }

    @Test fun englishSimilarGroupsKeepDecisionsAndAccessiblePreview()=screen {vm,api,language ->
        language(Locale.ENGLISH)
        compose.onNodeWithText("Menu").performClick()
        compose.onNodeWithText("Similar groups").performClick();idle(vm)
        compose.onNodeWithText("Same person?").assertIsDisplayed()
        compose.onNodeWithText("First group").assertIsDisplayed()
        compose.onNodeWithText("Second group").assertIsDisplayed()
        val preview=compose.onNodeWithTag("face-20").fetchSemanticsNode().config[SemanticsActions.CustomActions]
            .single {it.label=="Show original photo"}
        compose.runOnIdle {assertTrue(preview.action())}
        compose.onNodeWithText("Photos / Urlaub / Bild.jpg").assertIsDisplayed()
        compose.onNodeWithText("Close preview").performClick()
        compose.onNodeWithText("Help").performClick()
        compose.onNodeWithText("Review similar groups").assertIsDisplayed()
        compose.onNodeWithText("Close").performClick()
        compose.onNodeWithText("Keep separate").performClick();idle(vm)
        compose.onNodeWithText("No similar groups right now.").assertIsDisplayed()
        assertEquals(1,api.commits)
    }

    @Test fun errorFormattingPluralRulesAndLocaleConfigCoverBothLanguages() {
        val app=InstrumentationRegistry.getInstrumentation().targetContext
        val error=failureText(IOException("https://user:password@private.invalid?token=secret"),pending=true)
        val expected=listOf(Locale.GERMAN to "Serveranfrage [IO]",Locale.ENGLISH to "Server request [IO]")
        for((locale,prefix) in expected) {
            val context=app.createConfigurationContext(Configuration(app.resources.configuration).apply {setLocale(locale)})
            val text=UiStrings(context.resources)
            val displayed=text(error)
            assertTrue(displayed.startsWith(prefix))
            for(secret in listOf("password","private.invalid","secret")) assertFalse(displayed.contains(secret))
            assertEquals(if(locale==Locale.GERMAN)"1 Gesicht" else "1 face",text.faces(1))
            assertEquals(if(locale==Locale.GERMAN)"2 Gesichter" else "2 faces",text.faces(2))
            assertEquals(if(locale==Locale.GERMAN)"1 Gruppe" else "1 group",text.groups(1))
            assertEquals(if(locale==Locale.GERMAN)"2 Gruppen" else "2 groups",text.groups(2))
        }
        if(android.os.Build.VERSION.SDK_INT>=33) {
            val supported=android.app.LocaleConfig(app).supportedLocales!!
            assertEquals(setOf("de","en"),(0 until supported.size()).map {supported[it].language}.toSet())
        }
    }
}
