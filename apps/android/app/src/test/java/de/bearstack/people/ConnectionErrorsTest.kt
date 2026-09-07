package de.bearstack.people

import de.bearstack.people.connection.*
import de.bearstack.people.data.remote.ApiFailure
import java.io.IOException
import java.net.*
import java.security.cert.CertificateExpiredException
import javax.net.ssl.SSLHandshakeException
import javax.net.ssl.SSLPeerUnverifiedException
import org.junit.Assert.*
import org.junit.Test

class ConnectionErrorsTest {
    @Test fun firstConnectionDistinguishesNetworkFailuresWithoutClaimingPendingActions() {
        val cases=listOf(UnknownHostException() to "DNS", ConnectException() to "CONNECT",
            NoRouteToHostException() to "NO_ROUTE", SocketTimeoutException() to "TIMEOUT",
            SSLPeerUnverifiedException("secret") to "TLS_IDENTITY",SocketException("EPERM secret") to "NETWORK_PERMISSION")
        for((error,code) in cases) {
            val diagnostic=connectionDiagnostic(ConnectionAttemptException(ConnectionStage.CERTIFICATE_CHECK,error))!!
            assertEquals(code,diagnostic.code)
            assertTrue(diagnostic.text.contains("Zugangsdaten wurden noch nicht gesendet"))
            assertFalse(diagnostic.text.contains("offene Aktion"))
            assertFalse(diagnostic.text.contains("secret"))
        }
    }
    @Test fun unwrapsCertificateCauseAndPreservesApiStatusHandling() {
        val tls=SSLHandshakeException("private URL").apply {initCause(CertificateExpiredException())}
        assertEquals("CERT_EXPIRED",connectionDiagnostic(tls)!!.code)
        assertNull(connectionDiagnostic(ApiFailure(401,"","Anmeldung fehlgeschlagen")))
    }
    @Test fun onlyActuallyPendingActionsMentionReconciliationAndNoRawMessagesLeak() {
        val e=IOException("https://username:password@private.example/?token=secret")
        val diagnostic=connectionDiagnostic(e,true)!!
        assertTrue(diagnostic.text.contains("offene Aktion"))
        assertFalse(diagnostic.text.contains("password"));assertFalse(diagnostic.text.contains("private.example"))
        assertFalse(diagnostic.text.contains("secret"))
    }
}
