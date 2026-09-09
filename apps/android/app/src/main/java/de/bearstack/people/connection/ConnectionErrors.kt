package de.bearstack.people.connection

import de.bearstack.people.text.*
import de.bearstack.people.R
import de.bearstack.people.data.remote.ApiFailure
import java.io.IOException
import java.io.InterruptedIOException
import java.net.ConnectException
import java.net.NoRouteToHostException
import java.net.SocketException
import java.net.UnknownHostException
import java.security.cert.CertificateExpiredException
import java.security.cert.CertificateNotYetValidException
import javax.net.ssl.SSLException
import javax.net.ssl.SSLPeerUnverifiedException

enum class ConnectionStage { CERTIFICATE_CHECK, SIGN_IN, REQUEST }
class ConnectionAttemptException(val stage: ConnectionStage, cause: IOException) : IOException(cause)
data class ConnectionDiagnostic(val code: String, val stage: ConnectionStage, val text: UiText)

/** Only fixed diagnostics are exposed: exception messages can contain URLs and credentials. */
fun connectionDiagnostic(error: Throwable, pending: Boolean = false): ConnectionDiagnostic? {
    val causes = generateSequence(error) { it.cause }.take(12).toList()
    if (causes.any { it is ApiFailure } || causes.none { it is IOException }) return null
    val stage = causes.filterIsInstance<ConnectionAttemptException>().firstOrNull()?.stage ?: ConnectionStage.REQUEST
    val (code, detail) = when {
        causes.any { it is UnknownHostException } -> "DNS" to UiText(R.string.error_network_dns)
        causes.any { it is NoRouteToHostException } -> "NO_ROUTE" to UiText(R.string.error_network_route)
        causes.any { it is CertificateExpiredException } -> "CERT_EXPIRED" to UiText(R.string.error_certificate_expired)
        causes.any { it is CertificateNotYetValidException } -> "CERT_NOT_YET_VALID" to UiText(R.string.error_certificate_early)
        causes.any { it is SSLPeerUnverifiedException } -> "TLS_IDENTITY" to UiText(R.string.error_tls_identity)
        causes.any { it is SSLException } -> "TLS" to UiText(R.string.error_tls)
        causes.any { it is SocketException && (it.message.orEmpty().contains("EACCES") || it.message.orEmpty().contains("EPERM")) } ->
            "NETWORK_PERMISSION" to UiText(R.string.error_network_permission)
        causes.any { it is InterruptedIOException } -> "TIMEOUT" to UiText(R.string.error_network_timeout)
        causes.any { it is ConnectException } -> "CONNECT" to UiText(R.string.error_network_connect)
        else -> "IO" to UiText(R.string.error_network_io)
    }
    val prefix = when(stage) {
        ConnectionStage.CERTIFICATE_CHECK -> UiText(R.string.error_stage_certificate)
        ConnectionStage.SIGN_IN -> UiText(R.string.error_stage_sign_in)
        ConnectionStage.REQUEST -> UiText(R.string.error_stage_request)
    }
    val format=if(pending) R.string.error_diagnostic_pending
        else if(stage==ConnectionStage.CERTIFICATE_CHECK) R.string.error_diagnostic_credentials else R.string.error_diagnostic
    return ConnectionDiagnostic(code,stage,UiText(format,prefix,code,detail))
}
