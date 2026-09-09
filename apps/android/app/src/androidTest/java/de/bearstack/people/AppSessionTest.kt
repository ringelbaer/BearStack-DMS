package de.bearstack.people

import android.app.Application
import androidx.lifecycle.ViewModelStore
import androidx.test.platform.app.InstrumentationRegistry
import de.bearstack.people.connection.*
import de.bearstack.people.data.remote.ApiFailure
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
import java.util.concurrent.atomic.AtomicInteger

class AppSessionTest {
    @Test fun readerSessionRestoresAndSwitchesWithoutPeopleDatabase() = runBlocking {
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        val profiles = ProfileStore(context)
        profiles.clear()
        val server = MockWebServer()
        val cert = HeldCertificate.Builder().addSubjectAlternativeName("localhost").build()
        server.useHttps(HandshakeCertificates.Builder().heldCertificate(cert).build().sslSocketFactory(),false)
        val peopleRequests = AtomicInteger()
        server.dispatcher = object: Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                if (request.path.orEmpty().startsWith("/api/photos/labeling/v1/")) {
                    peopleRequests.incrementAndGet()
                    return MockResponse().setResponseCode(403)
                }
                if (!request.path.orEmpty().startsWith("/api/photos/v1/")) return MockResponse()
                val account = listOf("reader", "second").firstOrNull {
                    request.getHeader("Authorization") == Credentials.basic(it,"password")
                } ?: return MockResponse().setResponseCode(401)
                return MockResponse().setHeader("Content-Type","application/json").setBody(
                    if (request.path.orEmpty().contains("/session"))
                        """{"protocol":1,"can_manage_people":false,"instance":"instance","dataset":"dataset","account":"$account",
                            "settings":{"thumbnail_size":320,"folder_thumbnail_size":240,"preview_size":1280,"large_preview_size":2048,"slideshow_seconds":5,"frame_seconds":8}}"""
                    else """{"path":"","parent":"","page":1,"total":0,"has_next":false,"folder_total":0,"folder_has_next":false,"blog_has_next":false,"media":[],"folders":[],"blogs":[]}""")
            }
        }
        server.start()
        val address = server.url("/").toString()
        val scope = CoroutineScope(SupervisorJob()+Dispatchers.Main)
        var detachments = 0
        val session = AppSession(context,scope) { detachments++ }
        try {
            withContext(Dispatchers.Main) {
                assertFalse(session.restore())
                assertFalse(session.connect(address,"reader","password"))
                assertNotNull(session.certificate)
                assertTrue(session.confirmCertificate())
                val first = session.active!!
                assertNull(first.peopleSession)
                assertNotNull(first.photos)
                withTimeout(5000) { while (first.photos!!.state.value.loading) delay(10) }
                assertNull(first.photos!!.state.value.error)
                assertNull(session.certificate)
                assertFalse(session.connect(address,"invalid","password"))
                try { session.confirmCertificate(); fail("invalid account accepted") }
                catch (e: ApiFailure) { assertEquals(401,e.status) }
                assertSame("failed login replaced reader",first,session.active)
                assertEquals("reader",profiles.read()!!.username)
                session.cancelCertificate()
                assertFalse(session.connect(address,"second","password"))
                assertTrue(session.confirmCertificate())
                val second = session.active!!
                assertNotSame(first.images,second.images)
                assertNotSame(first.photos,second.photos)
                assertNotEquals(first.photos!!.session.scope,second.photos!!.session.scope)
                assertTrue(first.client.dispatcher.executorService.isShutdown)
                session.disconnect()
                assertTrue(second.client.dispatcher.executorService.isShutdown)
                assertNull(session.active)
                assertTrue(session.restore())
                assertEquals(second.photos!!.session.scope,session.active!!.photos!!.session.scope)
                session.disconnect(forget=true)
                assertNull(profiles.read())
                assertTrue(detachments>=4)
                assertEquals("reader login accessed person management",0,peopleRequests.get())

                // The application's existing facade must preserve the same
                // boundary, including cleanup after a reader-only login.
                val unusedDatabase = lazy<LabelingDatabase> { error("Reader opened the people database") }
                val models = ViewModelStore()
                val vm = PeopleViewModel(context.applicationContext as Application, unusedDatabase)
                models.put("reader",vm)
                suspend fun idle() { withTimeout(5000) { while(vm.state.value.busy) delay(10) } }
                try {
                    idle()
                    vm.connect(address,"reader","password"); idle()
                    vm.confirmCertificate(); idle()
                    assertNull(vm.state.value.error)
                    assertTrue(vm.state.value.connected)
                    assertNotNull(vm.photos)
                    assertFalse(vm.state.value.canManagePeople)
                    assertFalse(unusedDatabase.isInitialized())
                } finally { models.clear(); profiles.clear() }
            }
        } finally {
            withContext(Dispatchers.Main) { session.disconnect(forget=true) }
            scope.cancel()
            server.close()
        }
    }
}
