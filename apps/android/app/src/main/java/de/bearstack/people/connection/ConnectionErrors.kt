package de.bearstack.people.connection

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
data class ConnectionDiagnostic(val code: String, val stage: ConnectionStage, val text: String)

/** Only fixed diagnostics are exposed: exception messages can contain URLs and credentials. */
fun connectionDiagnostic(error: Throwable, pending: Boolean = false): ConnectionDiagnostic? {
    val causes = generateSequence(error) { it.cause }.take(12).toList()
    if (causes.any { it is ApiFailure } || causes.none { it is IOException }) return null
    val stage = causes.filterIsInstance<ConnectionAttemptException>().firstOrNull()?.stage ?: ConnectionStage.REQUEST
    val (code, detail) = when {
        causes.any { it is UnknownHostException } -> "DNS" to "Der Servername kann nicht aufgelöst werden. Adresse und DNS-Verbindung prüfen."
        causes.any { it is NoRouteToHostException } -> "NO_ROUTE" to "Keine Netzwerkroute zum Server. WLAN und VPN-Verbindung prüfen."
        causes.any { it is CertificateExpiredException } -> "CERT_EXPIRED" to "Das Serverzertifikat ist abgelaufen."
        causes.any { it is CertificateNotYetValidException } -> "CERT_NOT_YET_VALID" to "Das Serverzertifikat ist noch nicht gültig. Gerätezeit und Zertifikat prüfen."
        causes.any { it is SSLPeerUnverifiedException } -> "TLS_IDENTITY" to "Serveradresse und Zertifikatsidentität stimmen nicht überein. Hostname beziehungsweise IP-Eintrag im Zertifikat prüfen."
        causes.any { it is SSLException } -> "TLS" to "Die HTTPS-Prüfung ist fehlgeschlagen. Zertifikat, Fingerabdruck und HTTPS-Port prüfen."
        causes.any { it is SocketException && (it.message.orEmpty().contains("EACCES") || it.message.orEmpty().contains("EPERM")) } ->
            "NETWORK_PERMISSION" to "Android verweigert den Netzwerkzugriff. Netzwerkberechtigungen und VPN-Regeln für die App prüfen."
        causes.any { it is InterruptedIOException } -> "TIMEOUT" to "Der Server antwortet nicht rechtzeitig. Adresse, Port, WLAN, VPN und Firewall prüfen."
        causes.any { it is ConnectException } -> "CONNECT" to "Die Verbindung zum Server konnte nicht aufgebaut werden. Adresse, Port und Erreichbarkeit prüfen."
        else -> "IO" to "Die Netzwerkverbindung ist fehlgeschlagen. Erneut versuchen."
    }
    val prefix = when(stage) {
        ConnectionStage.CERTIFICATE_CHECK -> "HTTPS-Verbindungsprüfung"
        ConnectionStage.SIGN_IN -> "Anmeldung"
        ConnectionStage.REQUEST -> "Serveranfrage"
    }
    val suffix = if(pending) " Eine offene Aktion wird vor weiteren Änderungen geprüft."
        else if(stage == ConnectionStage.CERTIFICATE_CHECK) " Zugangsdaten wurden noch nicht gesendet." else ""
    return ConnectionDiagnostic(code,stage,"$prefix [$code]: $detail$suffix")
}
