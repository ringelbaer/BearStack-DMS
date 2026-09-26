package de.bearstack.people

import android.graphics.Bitmap
import android.util.Base64
import androidx.test.platform.app.InstrumentationRegistry
import coil3.decode.DataSource
import coil3.request.ErrorResult
import coil3.request.ImageRequest
import coil3.request.SuccessResult
import de.bearstack.people.connection.Connections
import de.bearstack.people.connection.Profile
import de.bearstack.people.data.remote.Photo
import de.bearstack.people.data.remote.PhotoSession
import de.bearstack.people.data.remote.PhotosApi
import de.bearstack.people.media.*
import de.bearstack.people.photos.photoPreviewRequest
import de.bearstack.people.ui.originalPhotoRequest
import java.io.ByteArrayOutputStream
import java.io.File
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.runBlocking
import okhttp3.Credentials
import okhttp3.mockwebserver.Dispatcher
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.RecordedRequest
import okhttp3.tls.HandshakeCertificates
import okhttp3.tls.HeldCertificate
import okio.Buffer
import org.junit.Assert.*
import org.junit.Test

class SessionImagesTest {
    private val context get() = InstrumentationRegistry.getInstrumentation().targetContext
    private val session = PhotoSession("account", false, 320, 240, 1280, 3072, 5, 8)
    private val photo = Photo("photo.png", "photo.png", "image", "image/png", "1", "2026-09-27", null, 100, 3072, 1536)

    private fun image(width: Int, height: Int): ByteArray {
        val bitmap = Bitmap.createBitmap(width, height, Bitmap.Config.ARGB_8888)
        return try {
            bitmap.eraseColor(android.graphics.Color.BLUE)
            ByteArrayOutputStream().also { bitmap.compress(Bitmap.CompressFormat.PNG, 100, it) }.toByteArray()
        } finally { bitmap.recycle() }
    }

    @Test fun authenticatedLargePreviewsAndFaceOriginalsLoadWithAndWithoutThumbnailCache() = runBlocking {
        val certificate = HeldCertificate.Builder().addSubjectAlternativeName("localhost").build()
        val server = MockWebServer().apply {
            useHttps(HandshakeCertificates.Builder().heldCertificate(certificate).build().sslSocketFactory(), false)
        }
        val large = image(3072, 1536)
        val small = image(64, 32)
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                if (request.getHeader("Authorization") != Credentials.basic("user", "password")) return MockResponse().setResponseCode(401)
                return MockResponse().setHeader("Content-Type", "image/png").setHeader("Cache-Control", "private, no-store")
                    .setBody(Buffer().write(if (request.requestUrl?.queryParameter("size") == "320") small else large))
            }
        }
        server.start()
        val pin = Base64.encodeToString(certificate.certificate.encoded, Base64.NO_WRAP)
        val client = Connections.client(Profile(server.url("/").toString(), "user", "password", pin))
        val service = PhotosApi(client, server.url("/").toString())
        try {
            for (withCache in listOf(true, false)) {
                val directory = File(context.cacheDir, "session-images-${System.nanoTime()}")
                val thumbnails = if (withCache) ThumbnailCache(ThumbnailDiskCache(directory, MIB), client, service, session, this) else null
                val loader = sessionImages(context, client, thumbnails)
                try {
                    val thumb = loader.execute(ImageRequest.Builder(context)
                        .data(if (withCache) cachedThumbnail(service, session, photo, 320) else service.thumbnail(photo, 320)).size(320).build())
                    assertTrue("small preview with cache=$withCache: $thumb", thumb is SuccessResult)
                    assertEquals(Credentials.basic("user", "password"), server.takeRequest(5, TimeUnit.SECONDS)?.getHeader("Authorization"))
                    val request = photoPreviewRequest(context, photo, service, session)
                    val result = loader.execute(request)
                    if (result is ErrorResult) throw AssertionError("large preview with cache=$withCache", result.throwable)
                    result as SuccessResult
                    assertEquals(DataSource.NETWORK, result.dataSource)
                    assertEquals(2048, result.image.width)
                    assertEquals(1024, result.image.height)
                    val recorded = server.takeRequest(5, TimeUnit.SECONDS)!!
                    assertEquals("3072", recorded.requestUrl?.queryParameter("size"))
                    assertEquals(Credentials.basic("user", "password"), recorded.getHeader("Authorization"))
                    assertEquals(DataSource.MEMORY_CACHE, (loader.execute(request) as SuccessResult).dataSource)
                    val original = loader.execute(originalPhotoRequest(context, server.url("/face/original").toString(), "account:original"))
                    if (original is ErrorResult) throw AssertionError("face original with cache=$withCache", original.throwable)
                    assertTrue(original is SuccessResult)
                    assertEquals(Credentials.basic("user", "password"), server.takeRequest(5, TimeUnit.SECONDS)?.getHeader("Authorization"))
                    if (thumbnails != null) assertEquals(small.size.toLong(), thumbnails.state.value.usage.bytes)
                } finally { loader.shutdown(); thumbnails?.close(clear = true); directory.deleteRecursively() }
            }
        } finally { Connections.close(client); server.close() }
    }

    @Test fun imageLoaderRejectsChangedCertificateBeforeSendingCredentials() = runBlocking {
        val approved = HeldCertificate.Builder().addSubjectAlternativeName("localhost").build()
        val changed = HeldCertificate.Builder().addSubjectAlternativeName("localhost").build()
        val server = MockWebServer().apply {
            useHttps(HandshakeCertificates.Builder().heldCertificate(changed).build().sslSocketFactory(), false)
            start()
        }
        val pin = Base64.encodeToString(approved.certificate.encoded, Base64.NO_WRAP)
        val client = Connections.client(Profile(server.url("/").toString(), "user", "password", pin))
        val loader = sessionImages(context, client, null)
        try {
            val result = loader.execute(originalPhotoRequest(context, server.url("/original").toString()))
            assertTrue(result is ErrorResult)
            assertEquals(0, server.requestCount)
        } finally { loader.shutdown(); Connections.close(client); server.close() }
    }
}
