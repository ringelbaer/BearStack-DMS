package de.bearstack.people

import android.app.Application
import androidx.lifecycle.ViewModelStore
import androidx.room.Room
import androidx.test.platform.app.InstrumentationRegistry
import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.createComposeRule
import de.bearstack.people.connection.ProfileStore
import de.bearstack.people.data.local.LabelingDatabase
import de.bearstack.people.people.PeopleViewModel
import kotlinx.coroutines.*
import okhttp3.Credentials
import okhttp3.mockwebserver.Dispatcher
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.RecordedRequest
import okhttp3.tls.HandshakeCertificates
import okhttp3.tls.HeldCertificate
import org.junit.Assert.*
import org.junit.Test
import org.junit.Rule
import de.bearstack.people.ui.PeopleApp

class LoginTest {
    @get:Rule val compose=createComposeRule()
    @Test fun accountSwitchPersistsNewProfileAndDistinguishesRejectedLogins() = runBlocking {
        val app=InstrumentationRegistry.getInstrumentation().targetContext.applicationContext as Application
        val profile=ProfileStore(app);profile.clear()
        val database=Room.inMemoryDatabaseBuilder(app,LabelingDatabase::class.java).build()
        val models=ViewModelStore()
        val certificate=HeldCertificate.Builder().addSubjectAlternativeName("localhost").build()
        val server=MockWebServer()
        server.useHttps(HandshakeCertificates.Builder().heldCertificate(certificate).build().sslSocketFactory(),false)
        server.dispatcher=object:Dispatcher() {
            override fun dispatch(request:RecordedRequest):MockResponse {
                val authorization=request.getHeader("Authorization")
                val username=listOf("first","second","reader","legacy","denied").firstOrNull {authorization==Credentials.basic(it,"password")}
                    ?: return MockResponse().setResponseCode(401)
                if(username=="denied") return MockResponse().setResponseCode(403)
                if(request.path!!.startsWith("/api/photos/v1/")) {
                    if(username=="legacy") return MockResponse().setResponseCode(404)
                    return MockResponse().setHeader("Content-Type","application/json").setBody(
                        if(request.path!!.contains("/session")) """{"protocol":1,"can_manage_people":${username!="reader"},"instance":"instance","dataset":"dataset","account":"$username",
                            "settings":{"thumbnail_size":320,"folder_thumbnail_size":240,"preview_size":1280,"large_preview_size":2048,"slideshow_seconds":5,"frame_seconds":8}}"""
                        else """{"path":"","parent":"","page":1,"total":0,"has_next":false,"folder_total":0,"folder_has_next":false,"blog_has_next":false,"media":[],"folders":[],"blogs":[]}""")
                }
                return MockResponse().setHeader("Content-Type","application/json").setBody(
                    if(request.path!!.contains("/session")) """{"protocol":1,"can_manage":true,"instance":"instance","dataset":"dataset","account":"$username","upper_id":0}"""
                    else """{"people":[],"next":0,"has_next":false}""")
            }
        }
        server.start()
        val address=server.url("/").toString()
        val vm=withContext(Dispatchers.Main) {PeopleViewModel(app,database).also {models.put("test",it)}}
        compose.setContent {PeopleApp(vm)}
        suspend fun idle() {withTimeout(15000) {while(vm.state.value.busy) delay(10)}}
        try {
            idle()
            for(username in listOf("first","second","reader","legacy","denied","invalid")) {
                withContext(Dispatchers.Main) {vm.connect(address,username,"password")};idle()
                assertNotNull("certificate: ${vm.state.value.error}",vm.state.value.certificate)
                withContext(Dispatchers.Main) {vm.confirmCertificate()};idle()
                when(username) {
                    "first","second","reader","legacy" -> {
                        assertNull(vm.state.value.error);assertTrue(vm.state.value.connected)
                        assertEquals(username!="legacy",vm.state.value.showGallery)
                        assertEquals(username!="reader",vm.state.value.canManagePeople)
                        assertEquals(username,profile.read()!!.username)
                        withContext(Dispatchers.Main) {vm.switchConnection()};idle()
                        assertNull("switch account",vm.state.value.error)
                        assertFalse(vm.state.value.connected);assertNull(profile.read())
                    }
                    "denied" -> {
                        assertEquals(R.string.error_photos_permission,vm.state.value.error!!.resource)
                        compose.onNode(hasText(de.bearstack.people.text.UiStrings(app.resources)(vm.state.value.error!!)) and hasAnyAncestor(isDialog())).assertIsDisplayed()
                    }
                    else -> {
                        assertEquals(R.string.error_auth,vm.state.value.error!!.resource)
                        compose.onNode(hasText(de.bearstack.people.text.UiStrings(app.resources)(vm.state.value.error!!)) and hasAnyAncestor(isDialog())).assertIsDisplayed()
                    }
                }
                withContext(Dispatchers.Main) {vm.cancelCertificate()}
            }
        } finally {withContext(Dispatchers.Main) {models.clear()};profile.clear();server.close()}
    }
}
