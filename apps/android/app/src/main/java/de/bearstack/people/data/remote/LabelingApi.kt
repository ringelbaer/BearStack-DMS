package de.bearstack.people.data.remote

import de.bearstack.people.connection.Connections
import java.io.IOException
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException
import kotlinx.coroutines.suspendCancellableCoroutine
import okhttp3.Call
import okhttp3.Callback
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONObject

data class Session(val instance: String, val dataset: String, val account: String, val upper: Long,
    val namedPeople: Boolean = false) {
    val scope: String get() = JSONObject().put("instance", instance).put("dataset", dataset).put("account", account).toString()
}
data class Person(val id: Long, val name: String, val revision: Long, val count: Long, val faceId: Long,
    val faces: List<Long> = emptyList(), val offset: Int = 0, val facePaths: Map<Long,String> = emptyMap(),
    val faceBounds: Map<Long,FaceBounds> = emptyMap(), val favorites: Set<Long> = emptySet())
data class Candidates(val people: List<Person>, val next: Long, val hasNext: Boolean)
data class Receipt(val operation: String, val action: String, val source: Long, val target: Long, val newId: Long,
    val faces: Long, val groups: Int, val at: Long)
class ApiFailure(val status: Int, val code: String, message: String) : IOException(message)

interface LabelingService {
    suspend fun session(): Session
    suspend fun candidates(after: Long, upper: Long): Candidates
    suspend fun namedPeople(after: Long, upper: Long): Candidates = throw ApiFailure(404,"not_found","Der Personenbereich benötigt BearStack 0.43.0.")
    suspend fun person(id: Long, offset: Int = 0): Person
    suspend fun suggestions(q: String, exact: Boolean = false): List<Person>
    suspend fun action(id: Long, body: String): Receipt
    suspend fun receipt(operation: String, dataset: String): Receipt
}
class LabelingApi(val client: OkHttpClient, address: String) : LabelingService {
    private val server = Connections.address(address)
    private val base = Connections.address(address).resolve("api/photos/labeling/v1/")!!
    fun image(face: Long, large: Boolean = false): String = base.resolve("faces/$face/thumbnail")!!.newBuilder()
        .addQueryParameter("size", if (large) "640" else "160").build().toString()
    fun original(face: Long): String = base.resolve("faces/$face/original")!!.toString()
    fun gallery(name: String): String = server.resolve("photos")!!.newBuilder()
        // The gallery tokenizer concatenates quoted segments; backslashes are literal.
        .addQueryParameter("q", "person:\"${name.replace("\"", "\"'\"'\"")}\"").build().toString()
    private suspend fun json(path: String, query: Map<String,String> = emptyMap(), body: String? = null): JSONObject {
        val url = base.resolve(path)!!.newBuilder().apply { query.forEach { (k,v) -> addQueryParameter(k,v) } }.build()
        val request = Request.Builder().url(url).apply { body?.let { post(it.toRequestBody("application/json".toMediaType())) } }.build()
        return suspendCancellableCoroutine { continuation ->
            val call = client.newCall(request)
            continuation.invokeOnCancellation { call.cancel() }
            call.enqueue(object : Callback {
                override fun onFailure(call: Call, e: IOException) { if (!continuation.isCancelled) continuation.resumeWithException(e) }
                override fun onResponse(call: Call, response: Response) {
                    response.use {
                        try {
                            val responseBody = it.body ?: throw IOException("Leere Serverantwort")
                            val source = responseBody.source()
                            if (source.request(256 * 1024L + 1)) throw IOException("Serverantwort überschreitet das Größenlimit")
                            val text = source.readUtf8()
                            if (!it.isSuccessful) {
                                val code = runCatching { JSONObject(text).optString("code") }.getOrDefault("")
                                val message = when(it.code) {
                                    401 -> "Anmeldung abgelaufen oder Zugangsdaten falsch."
                                    403 -> "Das Konto benötigt das Recht zur Personenverwaltung."
                                    404 -> "Datensatz oder API nicht vorhanden (BearStack ab 0.30.0 erforderlich)."
                                    409 -> if (code == "name_exists") "Dieser Name existiert bereits." else "Die Personengruppe wurde geändert. Bitte erneut prüfen."
                                    else -> "Serverfehler (${it.code}). Erneut versuchen."
                                }
                                throw ApiFailure(it.code, code, message)
                            }
                            val result = JSONObject(text)
                            if (!continuation.isCancelled) continuation.resume(result)
                        } catch (e: Exception) { if (!continuation.isCancelled) continuation.resumeWithException(e) }
                    }
                }
            })
        }
    }
    override suspend fun session(): Session {
        val o = json("session")
        require(o.getInt("protocol") == 1 && o.getBoolean("can_manage")) { "Inkompatible API oder fehlende Personenrechte." }
        return Session(o.getString("instance"),o.getString("dataset"),o.getString("account"),o.getLong("upper_id"),o.optBoolean("named_people"))
    }
    override suspend fun candidates(after: Long, upper: Long): Candidates {
        val o = json("candidates", mapOf("after" to "$after", "upper" to "$upper"))
        return Candidates(people(o),o.getLong("next"),o.getBoolean("has_next"))
    }
    override suspend fun namedPeople(after: Long, upper: Long): Candidates {
        val o = json("people", mapOf("after" to "$after", "upper" to "$upper"))
        return Candidates(people(o),o.getLong("next"),o.getBoolean("has_next"))
    }
    override suspend fun person(id: Long, offset: Int): Person = person(json("people/$id",mapOf("offset" to "$offset")))
    override suspend fun suggestions(q: String, exact: Boolean): List<Person> = people(json("suggestions",mapOf("q" to q,"exact" to if(exact) "1" else "0")))
    override suspend fun action(id: Long, body: String): Receipt = receipt(json("people/$id/actions", body=body))
    override suspend fun receipt(operation: String, dataset: String): Receipt = receipt(json("actions/$operation",mapOf("dataset" to dataset)))
    private fun people(o: JSONObject): List<Person> = o.getJSONArray("people").let { a -> List(a.length()) { person(a.getJSONObject(it)) } }
    internal fun person(o: JSONObject): Person {
        val faces = o.optJSONArray("faces")
        return Person(o.getLong("id"),o.getString("name"),o.getLong("revision"),o.getLong("count"),o.getLong("face_id"),
            if(faces == null) emptyList() else List(faces.length()) { faces.getJSONObject(it).getLong("id") },o.optInt("offset"),
            if(faces == null) emptyMap() else (0 until faces.length()).associate {
                val face=faces.getJSONObject(it);face.getLong("id") to face.optString("display_path", "")
            }, if(faces == null) emptyMap() else (0 until faces.length()).mapNotNull {
                val face=faces.getJSONObject(it)
                val b=face.optJSONObject("bounds") ?: return@mapNotNull null
                FaceBounds.validated(b.optDouble("x").toFloat(), b.optDouble("y").toFloat(),
                    b.optDouble("width").toFloat(), b.optDouble("height").toFloat())?.let { face.getLong("id") to it }
            }.toMap(), if(faces == null) emptySet() else (0 until faces.length()).mapNotNull {
                faces.getJSONObject(it).takeIf { face -> face.optBoolean("favorite") }?.getLong("id")
            }.toSet())
    }
    private fun receipt(o: JSONObject) = Receipt(o.getString("operation_id"),o.getString("action"),o.getLong("source_id"),
        o.getLong("target_id"),o.getLong("new_id"),o.getLong("faces"),o.getInt("groups"),o.getLong("at"))
}
