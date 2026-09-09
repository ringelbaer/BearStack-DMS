package de.bearstack.people.data.remote

import de.bearstack.people.text.*
import de.bearstack.people.R
import java.io.IOException
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.NonCancellable
import kotlinx.coroutines.withContext
import okhttp3.Call
import okhttp3.Callback
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import org.json.JSONObject

// One cancellable, bounded transport for both native API families. The caller
// owns the authenticated client; neither API follows URLs from JSON responses.
internal suspend fun OkHttpClient.json(request: Request, maxBytes: Long,
    errorMessage: (Int, String) -> UiText): JSONObject = readResponse(request) { response, _ ->
    val source = (response.body ?: throw UserIoFailure(UiText(R.string.error_response_empty))).source()
    if (source.request(maxBytes + 1)) throw UserIoFailure(UiText(R.string.error_response_size))
    val text = source.readUtf8()
    if (!response.isSuccessful) {
        val code = runCatching { JSONObject(text).optString("code") }.getOrDefault("")
        throw ApiFailure(response.code, code, errorMessage(response.code, code))
    }
    try {JSONObject(text)} catch(_: org.json.JSONException) {throw UserIoFailure(UiText(R.string.error_response_invalid))}
}

internal suspend fun <T> OkHttpClient.readResponse(request: Request, waitForClose: Boolean = false,
    read: (Response, () -> Boolean) -> T): T {
    val closed=CompletableDeferred<Unit>()
    try {
        return suspendCancellableCoroutine { continuation ->
            val call = newCall(request)
            continuation.invokeOnCancellation { call.cancel() }
            val callback=object : Callback {
                override fun onFailure(call: Call, e: IOException) {
                    closed.complete(Unit)
                    if (!continuation.isCancelled) continuation.resumeWithException(e)
                }
                override fun onResponse(call: Call, response: Response) {
                    try {
                        val result=response.use { read(it) {continuation.isActive} }
                        if (!continuation.isCancelled) continuation.resume(result)
                    } catch (e: Exception) {
                        if (!continuation.isCancelled) continuation.resumeWithException(e)
                    } finally { closed.complete(Unit) }
                }
            }
            try { call.enqueue(callback) }
            catch(e: Exception) {
                closed.complete(Unit)
                if(!continuation.isCancelled) continuation.resumeWithException(e)
            }
        }
    } finally {
        // A document cannot be removed while a cancelled callback still writes
        // its final buffer. JSON calls do not need this cleanup barrier.
        if(waitForClose) withContext(NonCancellable) {closed.await()}
    }
}
