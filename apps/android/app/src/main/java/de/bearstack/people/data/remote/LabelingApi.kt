package de.bearstack.people.data.remote

import de.bearstack.people.text.*
import de.bearstack.people.R
import de.bearstack.people.connection.Connections
import java.io.IOException
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.flow
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONObject

data class Session(val instance: String, val dataset: String, val account: String, val upper: Long,
    val namedPeople: Boolean = false, val namedSearch: Boolean = false, val mergeSuggestions: Boolean = false, val mergeNaming: Boolean = false,
    val manualMerge: Boolean = false, val mergeSideActions: Boolean = false) {
    val scope: String get() = JSONObject().put("instance", instance).put("dataset", dataset).put("account", account).toString()
}
data class Person(val id: Long, val name: String, val revision: Long, val count: Long, val faceId: Long,
    val faces: List<Long> = emptyList(), val offset: Int = 0, val facePaths: Map<Long,String> = emptyMap(),
    val faceBounds: Map<Long,FaceBounds> = emptyMap(), val favorites: Set<Long> = emptySet(),
    val originalKeys: Map<Long,String> = emptyMap())
data class FaceMatch(val id: Long, val name: String, val count: Long, val faceId: Long)

data class Candidates(val people: List<Person>, val next: Long, val hasNext: Boolean)
data class MergeSuggestion(val id: Long, val source: Person, val target: Person, val score: Double? = null)
data class Receipt(val operation: String, val action: String, val source: Long, val target: Long, val newId: Long,
    val faces: Long, val groups: Int, val at: Long, val sourceRevision: Long = 0)
class ApiFailure(val status: Int, val code: String, message: String,
    override val userText: UiText = when {
        status==401 -> UiText(R.string.error_auth)
        status==403 -> UiText(R.string.error_people_permission)
        code=="name_exists" -> UiText(R.string.error_name_exists)
        status==409 -> UiText(R.string.error_group_changed)
        else -> UiText(R.string.error_server,status)
    }) : IOException(message),DescribedFailure {
    constructor(status: Int,code: String,text: UiText):this(status,code,"API failure ($status)",text)
}

interface LabelingService {
    suspend fun session(): Session
    suspend fun candidates(after: Long, upper: Long): Candidates
    suspend fun namedPeople(after: Long, upper: Long): Candidates = throw ApiFailure(404,"not_found",UiText(R.string.error_people_version))
    suspend fun searchPeople(after: Long, upper: Long, q: String): Candidates =
        if(q.isBlank()) namedPeople(after,upper) else throw ApiFailure(404,"not_found",UiText(R.string.error_search_version))
    suspend fun personFaces(id: Long, offset: Int, after: Long): Person = person(id,offset)
    suspend fun person(id: Long, offset: Int = 0): Person
    suspend fun suggestions(q: String, exact: Boolean = false): List<Person>
    suspend fun mergeGroups(after: Long, upper: Long, includeNamed: Boolean): Candidates =
        throw ApiFailure(404,"not_found",UiText(R.string.people_manual_merge_version))
    fun faceMatches(face: Long): Flow<List<FaceMatch>> = flow { throw ApiFailure(404,"not_found",UiText(R.string.people_face_search_unavailable)) }
    suspend fun nextMergeSuggestion(excluded: Pair<Long,Long>? = null): MergeSuggestion? = throw ApiFailure(404,"not_found",UiText(R.string.error_merge_version))
    suspend fun action(id: Long, body: String): Receipt
    suspend fun receipt(operation: String, dataset: String): Receipt
}
class LabelingApi(val client: OkHttpClient, address: String) : LabelingService {
    // A progressing search may outlast the ordinary request deadline. Keep the
    // authenticated client's connection/read timeouts and shared connection pool.
    private val faceMatchClient = client.newBuilder().callTimeout(0, TimeUnit.MILLISECONDS).build()
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
        return client.json(request, 256 * 1024L) { status, code ->
            when(status) {
                401 -> UiText(R.string.error_auth)
                403 -> UiText(R.string.error_people_permission)
                404 -> UiText(R.string.error_people_missing)
                409 -> if (code == "name_exists") UiText(R.string.error_name_exists) else UiText(R.string.error_group_changed)
                else -> UiText(R.string.error_server,status)
            }
        }
    }

    override suspend fun session(): Session {
        val o = json("session")
        requireMessage(o.getInt("protocol") == 1 && o.getBoolean("can_manage"),R.string.error_people_protocol)
        return Session(o.getString("instance"),o.getString("dataset"),o.getString("account"),o.getLong("upper_id"),o.optBoolean("named_people"),o.optBoolean("named_search"),o.optBoolean("merge_suggestions"),o.optBoolean("merge_naming"),o.optBoolean("manual_merge"),o.optBoolean("merge_side_actions"))
    }
    override suspend fun candidates(after: Long, upper: Long): Candidates {
        val o = json("candidates", mapOf("after" to "$after", "upper" to "$upper"))
        return Candidates(people(o),o.getLong("next"),o.getBoolean("has_next"))
    }
    override suspend fun mergeGroups(after: Long, upper: Long, includeNamed: Boolean): Candidates {
        val o=json("groups",mapOf("after" to "$after","upper" to "$upper","include_named" to if(includeNamed) "1" else "0"))
        val page=Candidates(people(o),o.getLong("next"),o.getBoolean("has_next"))
        requireMessage(page.people.size<=20 && (!page.hasNext || page.people.size==20),R.string.error_response_invalid)
        var previous=after
        page.people.forEach {p ->
            requireMessage(p.id>previous && p.id<=upper && p.revision>0 && p.count>0 && p.faceId>0 &&
                (includeNamed || p.name.isEmpty()) && p.faces.size<=1 && p.faces.all {it==p.faceId},R.string.error_response_invalid)
            previous=p.id
        }
        requireMessage(page.next==previous,R.string.error_response_invalid)
        return page
    }
    override suspend fun namedPeople(after: Long, upper: Long): Candidates {
        val o = json("people", mapOf("after" to "$after", "upper" to "$upper"))
        return Candidates(people(o),o.getLong("next"),o.getBoolean("has_next"))
    }
    override suspend fun searchPeople(after: Long, upper: Long, q: String): Candidates {
        val o=json("people",mapOf("after" to "$after","upper" to "$upper","q" to q))
        return Candidates(people(o),o.getLong("next"),o.getBoolean("has_next"))
    }
    override suspend fun personFaces(id: Long, offset: Int, after: Long): Person = person(json("people/$id",
        mapOf("offset" to "$offset","after_face" to "$after","limit" to "40")))
    override suspend fun person(id: Long, offset: Int): Person = person(json("people/$id",mapOf("offset" to "$offset")))
    override suspend fun suggestions(q: String, exact: Boolean): List<Person> = people(json("suggestions",mapOf("q" to q,"exact" to if(exact) "1" else "0")))
    override fun faceMatches(face: Long): Flow<List<FaceMatch>> {
        require(face > 0)
        val request = Request.Builder().url(server.resolve("photos/faces/$face/suggestions")!!)
            .header("Accept", "application/x-ndjson").build()
        return faceMatchClient.faceMatchUpdates(request)
    }
    override suspend fun nextMergeSuggestion(excluded: Pair<Long,Long>?): MergeSuggestion? = json("merge-suggestions/next",
        excluded?.let {mapOf("exclude_source" to "${it.first}","exclude_target" to "${it.second}")} ?: emptyMap()).optJSONObject("suggestion")?.let {
        MergeSuggestion(it.getLong("id"),person(it.getJSONObject("source")),person(it.getJSONObject("target")),
            it.optDouble("score",Double.NaN).takeIf {score -> score.isFinite()})
    }
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
            }.toSet(), if(faces == null) emptyMap() else (0 until faces.length()).mapNotNull {
                val face=faces.getJSONObject(it)
                face.optString("original_key").takeIf { key -> key.matches(Regex("[a-f0-9]{64}")) }?.let { face.getLong("id") to it }
            }.toMap())
    }
    private fun receipt(o: JSONObject) = Receipt(o.getString("operation_id"),o.getString("action"),o.getLong("source_id"),
        o.getLong("target_id"),o.getLong("new_id"),o.getLong("faces"),o.getInt("groups"),o.getLong("at"),o.optLong("source_revision"))
}
