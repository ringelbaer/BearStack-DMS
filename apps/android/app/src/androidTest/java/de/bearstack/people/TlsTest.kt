package de.bearstack.people

import de.bearstack.people.connection.*
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.tls.HandshakeCertificates
import okhttp3.tls.HeldCertificate
import okhttp3.Request
import org.junit.Assert.*
import org.junit.Test
import java.util.concurrent.TimeUnit

class TlsTest {
    @Test fun certificateProbeSendsNoAuthorizationAndPinnedClientDoesNotFollowRedirects() = runBlocking {
        val cert=HeldCertificate.Builder().addSubjectAlternativeName("localhost").build()
        val server=MockWebServer()
        server.useHttps(HandshakeCertificates.Builder().heldCertificate(cert).build().sslSocketFactory(),false)
        server.start()
        val other=MockWebServer();other.start()
        try {
            server.enqueue(MockResponse().setResponseCode(401))
            val address=server.url("/").toString()
            val offer=Connections.inspect(address)!!
            assertNull(server.takeRequest(3,TimeUnit.SECONDS)!!.getHeader("Authorization"))
            val client=Connections.client(Profile(address,"user","password",offer.encoded))
            try {
                server.enqueue(MockResponse().setResponseCode(302).setHeader("Location",other.url("/")))
                client.newCall(Request.Builder().url(address).build()).execute().use { assertEquals(302,it.code) }
                assertTrue(server.takeRequest(3,TimeUnit.SECONDS)!!.getHeader("Authorization")!!.startsWith("Basic "))
                assertEquals(0,other.requestCount)
            } finally {client.connectionPool.evictAll();client.dispatcher.executorService.shutdown()}
        } finally {server.close();other.close()}
    }
    @Test fun expiredAndWrongHostnameCertificatesCannotBeOffered() = runBlocking {
        val expired=HeldCertificate.Builder().addSubjectAlternativeName("localhost").validityInterval(1,2).build()
        val wrongHost=HeldCertificate.Builder().addSubjectAlternativeName("wrong.invalid").build()
        for(cert in listOf(expired,wrongHost)) {
            val server=MockWebServer()
            server.useHttps(HandshakeCertificates.Builder().heldCertificate(cert).build().sslSocketFactory(),false)
            server.start()
            try {
                server.enqueue(MockResponse().setResponseCode(401))
                try { Connections.inspect(server.url("/").toString());fail("invalid certificate offered") } catch(_:javax.net.ssl.SSLException) {}
                assertEquals(0,server.requestCount)
            } finally {server.close()}
        }
    }
}
