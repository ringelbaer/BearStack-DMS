package de.bearstack.people.data.remote

import de.bearstack.people.R
import de.bearstack.people.text.*
import java.io.EOFException
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.buffer
import kotlinx.coroutines.flow.channelFlow
import okhttp3.OkHttpClient
import okhttp3.Request
import org.json.JSONException
import org.json.JSONObject

private const val MAX_FACE_MATCH_BYTES = 64 * 1024L

// Each event replaces the entire ranking. Keep only the newest pending snapshot
// so a busy UI cannot accumulate results or block the server's reference cache.
internal fun OkHttpClient.faceMatchUpdates(request: Request): Flow<List<FaceMatch>> = channelFlow {
    readResponse(request) { response, active ->
        val body = response.body ?: throw UserIoFailure(UiText(R.string.error_response_empty))
        val source = body.source()
        val streaming = body.contentType()?.let { it.type == "application" && it.subtype == "x-ndjson" } == true
        if (!response.isSuccessful || !streaming) {
            // Older servers return one JSON ranking even when streaming is requested.
            if (source.request(MAX_FACE_MATCH_BYTES + 1)) throw UserIoFailure(UiText(R.string.error_response_size))
            val text = source.readUtf8()
            if (!response.isSuccessful) {
                val code = runCatching { JSONObject(text).optString("code") }.getOrDefault("")
                val message = when (response.code) {
                    401 -> UiText(R.string.error_auth)
                    403 -> UiText(R.string.error_people_permission)
                    404 -> UiText(R.string.people_face_search_unavailable)
                    409 -> UiText(R.string.error_group_changed)
                    else -> UiText(R.string.error_server, response.code)
                }
                throw ApiFailure(response.code, code, message)
            }
            trySend(faceMatchRanking(faceMatchObject(text))).getOrThrow()
        } else {
            while (active()) {
                val line = try { source.readUtf8LineStrict(MAX_FACE_MATCH_BYTES) }
                catch (_: EOFException) {
                    // EOF without a final event is a failed search, never an empty success.
                    throw UserIoFailure(UiText(if (source.buffer.size > MAX_FACE_MATCH_BYTES)
                        R.string.error_response_size else R.string.error_response_invalid))
                }
                if (line.isBlank()) continue
                val event = faceMatchObject(line)
                if (event.has("error") && event.opt("error") != "")
                    throw UserIoFailure(UiText(R.string.people_face_search_failed))
                val done = event.opt("done") as? Boolean
                    ?: throw UserIoFailure(UiText(R.string.error_response_invalid))
                trySend(faceMatchRanking(event)).getOrThrow()
                if (done) break
            }
        }
    }
}.buffer(Channel.CONFLATED)

private fun faceMatchObject(text: String): JSONObject = try { JSONObject(text) }
catch (_: JSONException) { throw UserIoFailure(UiText(R.string.error_response_invalid)) }

private fun faceMatchRanking(event: JSONObject): List<FaceMatch> = try {
    val people = event.getJSONArray("people")
    requireMessage(people.length() <= 20, R.string.error_response_invalid)
    List(people.length()) { i -> people.getJSONObject(i).let {
        FaceMatch(it.getLong("id"), it.getString("name"), it.getLong("count"), it.getLong("face_id"))
    } }.onEach {
        requireMessage(it.id > 0 && it.faceId > 0 && it.name.isNotBlank() && it.count > 0, R.string.error_response_invalid)
    }.also { requireMessage(it.map { match -> match.id }.distinct().size == it.size, R.string.error_response_invalid) }
} catch (_: JSONException) { throw UserIoFailure(UiText(R.string.error_response_invalid)) }
