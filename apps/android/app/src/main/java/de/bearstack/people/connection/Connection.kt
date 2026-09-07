package de.bearstack.people.connection

import android.content.Context
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.AtomicFile
import android.util.Base64
import java.io.File
import java.security.KeyStore
import java.security.MessageDigest
import java.security.cert.CertificateException
import java.security.cert.X509Certificate
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec
import javax.net.ssl.SSLContext
import javax.net.ssl.TrustManagerFactory
import javax.net.ssl.X509TrustManager
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.Credentials
import okhttp3.HttpUrl
import okhttp3.HttpUrl.Companion.toHttpUrl
import okhttp3.OkHttpClient
import okhttp3.Request
import org.json.JSONObject
import java.util.concurrent.TimeUnit

// No credentials, certificate or images are included in Android backups.
data class Profile(val url: String, val username: String, val password: String, val certificate: String = "")
data class CertificateOffer(val encoded: String, val fingerprint: String, val subject: String, val expires: String)

object Connections {
    fun address(raw: String): HttpUrl {
        val url = raw.trim().trimEnd('/').plus('/').toHttpUrl()
        require(url.isHttps && url.username.isEmpty() && url.password.isEmpty() && url.query == null && url.fragment == null) {
            "Eine HTTPS-Adresse ohne Zugangsdaten, Abfrage oder Fragment eingeben."
        }
        return url
    }
    private fun systemTrust(): X509TrustManager = TrustManagerFactory.getInstance(TrustManagerFactory.getDefaultAlgorithm()).apply {
        init(null as KeyStore?)
    }.trustManagers.filterIsInstance<X509TrustManager>().single()

    internal fun trust(pinned: String = "", offer: ((X509Certificate) -> Unit)? = null): X509TrustManager {
        val system = systemTrust()
        return object : X509TrustManager {
            override fun getAcceptedIssuers(): Array<X509Certificate> = system.acceptedIssuers
            override fun checkClientTrusted(chain: Array<X509Certificate>, type: String) = system.checkClientTrusted(chain, type)
            override fun checkServerTrusted(chain: Array<X509Certificate>, type: String) {
                if (chain.isEmpty()) throw CertificateException("Kein Serverzertifikat")
                chain[0].checkValidity()
                if (pinned.isNotEmpty()) {
                    if (Base64.encodeToString(chain[0].encoded, Base64.NO_WRAP) != pinned)
                        throw CertificateException("Serverzertifikat geändert. Verbindung erneut einrichten und Fingerabdruck prüfen.")
                    return
                }
                try { system.checkServerTrusted(chain, type) } catch (e: CertificateException) {
                    // Only a self-signed leaf may be offered. Hostname validation still belongs to OkHttp.
                    if (offer == null || chain.size != 1) throw e
                    try { chain[0].verify(chain[0].publicKey) } catch (_: Exception) { throw e }
                    offer(chain[0])
                }
            }
        }
    }
    private fun builder(trust: X509TrustManager): OkHttpClient.Builder {
        val tls = SSLContext.getInstance("TLS").apply { init(null, arrayOf(trust), null) }
        return OkHttpClient.Builder().sslSocketFactory(tls.socketFactory, trust)
            .followRedirects(false).followSslRedirects(false).retryOnConnectionFailure(false)
            .connectTimeout(15, TimeUnit.SECONDS).readTimeout(30, TimeUnit.SECONDS).callTimeout(45, TimeUnit.SECONDS)
    }
    suspend fun inspect(raw: String): CertificateOffer? = withContext(Dispatchers.IO) {
        val base = address(raw)
        var cert: X509Certificate? = null
        val client = builder(trust(offer = { cert = it })).build()
        try {
            // This request is deliberately unauthenticated, including on a newly observed certificate.
            client.newCall(Request.Builder().url(base.resolve("api/photos/labeling/v1/session")!!).build()).execute().use { }
            cert?.let {
                CertificateOffer(Base64.encodeToString(it.encoded, Base64.NO_WRAP),
                    MessageDigest.getInstance("SHA-256").digest(it.encoded).joinToString(":") { b -> "%02X".format(b) },
                    it.subjectX500Principal.name, it.notAfter.toString())
            }
        } catch (e: java.io.IOException) {
            throw ConnectionAttemptException(ConnectionStage.CERTIFICATE_CHECK,e)
        } finally { client.connectionPool.evictAll(); client.dispatcher.executorService.shutdown() }
    }
    fun client(profile: Profile): OkHttpClient {
        val base = address(profile.url)
        require(profile.username.isNotBlank() && ':' !in profile.username && profile.username.none { it.isISOControl() }) { "Ungültiger Benutzername" }
        return builder(trust(profile.certificate)).addInterceptor { chain ->
            val request = chain.request()
            check(request.url.scheme == base.scheme && request.url.host == base.host && request.url.port == base.port &&
                request.url.encodedPath.startsWith(base.encodedPath)) { "Anfrage außerhalb der BearStack-Instanz" }
            chain.proceed(request.newBuilder().header("Authorization", Credentials.basic(profile.username, profile.password, Charsets.UTF_8))
                .header("Accept", "application/json").build())
        }.build()
    }
}

class ProfileStore(context: Context) {
    private val file = AtomicFile(File(context.noBackupFilesDir, "connection.enc"))
    private fun key(): SecretKey {
        val store = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        (store.getKey("bearstack.connection", null) as? SecretKey)?.let { return it }
        return KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore").apply {
            init(KeyGenParameterSpec.Builder("bearstack.connection", KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT)
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM).setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE).build())
        }.generateKey()
    }
    suspend fun read(): Profile? = withContext(Dispatchers.IO) {
        if (!file.baseFile.exists()) return@withContext null
        val bytes = file.readFully()
        require(bytes.size > 28) { "Gespeicherte Verbindung ist beschädigt" }
        val cipher = Cipher.getInstance("AES/GCM/NoPadding").apply { init(Cipher.DECRYPT_MODE, key(), GCMParameterSpec(128, bytes.copyOfRange(0,12))) }
        val json = JSONObject(String(cipher.doFinal(bytes.copyOfRange(12,bytes.size)), Charsets.UTF_8))
        Profile(json.getString("url"), json.getString("username"), json.getString("password"), json.optString("certificate"))
    }
    suspend fun write(p: Profile) = withContext(Dispatchers.IO) {
        val cipher = Cipher.getInstance("AES/GCM/NoPadding").apply { init(Cipher.ENCRYPT_MODE, key()) }
        val json = JSONObject().put("url",p.url).put("username",p.username).put("password",p.password).put("certificate",p.certificate)
        val encrypted = cipher.iv + cipher.doFinal(json.toString().toByteArray(Charsets.UTF_8))
        val stream = file.startWrite()
        try { stream.write(encrypted); file.finishWrite(stream) } catch (e: Exception) { file.failWrite(stream); throw e }
    }
    suspend fun clear() = withContext(Dispatchers.IO) { file.delete() }
}
