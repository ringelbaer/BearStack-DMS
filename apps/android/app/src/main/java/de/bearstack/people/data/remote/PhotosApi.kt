package de.bearstack.people.data.remote

import de.bearstack.people.text.*
import de.bearstack.people.R
import de.bearstack.people.connection.Connections
import okhttp3.OkHttpClient
import okhttp3.Request
import org.json.JSONArray
import org.json.JSONObject
import java.io.IOException
import java.io.OutputStream
import java.util.concurrent.TimeUnit

data class PhotoSession(val scope: String, val canManagePeople: Boolean, val thumbnailSize: Int,
    val folderThumbnailSize: Int, val previewSize: Int, val largePreviewSize: Int,
    val slideshowSeconds: Int, val frameSeconds: Int)
data class Photo(val path: String, val name: String, val type: String, val mime: String, val version: String,
    val modified: String, val captured: String?, val bytes: Long, val width: Int, val height: Int,
    val camera: String = "", val lens: String = "", val latitude: Double? = null, val longitude: Double? = null,
    val rating: Double? = null, val tags: List<String> = emptyList(), val keywords: List<String> = emptyList(),
    val people: List<String> = emptyList()) {
    val date: String get() = captured ?: modified
}
data class PhotoFolder(val path: String, val name: String, val date: String?, val count: Int,
    val approximate: Boolean, val folders: Int, val previews: List<Photo>)
data class PhotoBlog(val path: String, val name: String, val date: String?, val modified: String,
    val text: String = "", val html: String = "")
data class PhotoQuery(val path: String = "", val query: String = "", val recursive: Boolean = false,
    val gps: Boolean = false, val sort: String = "descending_date", val type: String = "")
data class PhotoPage(val path: String, val parent: String, val page: Int, val total: Int, val hasNext: Boolean,
    val folderTotal: Int, val folderHasNext: Boolean, val blogHasNext: Boolean,
    val media: List<Photo>, val folders: List<PhotoFolder>, val blogs: List<PhotoBlog>)
data class PhotoMapBounds(val south: Double, val west: Double, val north: Double, val east: Double)
data class PhotoMapMarker(val latitude: Double, val longitude: Double, val count: Int, val path: String = "",val bounds: PhotoMapBounds? = null)
data class PhotoMapData(val total: Int, val bounds: PhotoMapBounds?, val markers: List<PhotoMapMarker>)
data class PhotoMapPage(val total: Int,val page: Int,val hasNext: Boolean,val media: List<Photo>)
data class PhotoDatePosition(val path: String, val date: String, val page: Int)

data class PhotoTrackFile(val path: String,val name: String,val modified: String,val bytes: Long)
data class PhotoTrackPage(val tracks: List<PhotoTrackFile>,val cursor: String,val previousCursor: String,
    val hasNext: Boolean,val hasPrevious: Boolean,val ready: Boolean)
data class PhotoMapPoint(val latitude: Double,val longitude: Double)
data class PhotoTrackGeometry(val path: String,val name: String,val bounds: PhotoMapBounds?,
    val segments: List<List<PhotoMapPoint>>,val totalPoints: Int,val simplified: Boolean,val omittedSegments: Int)
data class PhotoRouteData(val geometry: PhotoTrackGeometry,val totalMedia: Int,val radiusMeters: Int)

interface PhotosService {
    suspend fun session(): PhotoSession
    suspend fun browse(query: PhotoQuery, page: Int = 1, section: String = ""): PhotoPage
    suspend fun locateDate(date: String): PhotoDatePosition =
        throw UserIoFailure(UiText(R.string.photos_date_unavailable))
    suspend fun info(path: String): Photo
    suspend fun blog(path: String): PhotoBlog
    suspend fun map(query: PhotoQuery, bounds: PhotoMapBounds? = null): PhotoMapData =
        throw UnsupportedOperationException("Map unavailable")
    suspend fun mapMedia(query: PhotoQuery,bounds: PhotoMapBounds,page: Int): PhotoMapPage =
        throw UnsupportedOperationException("Map media unavailable")
    suspend fun tracks(path: String,cursor: String = "",before: Boolean = false): PhotoTrackPage =
        throw UnsupportedOperationException("Tracks unavailable")
    suspend fun track(path: String,bounds: PhotoMapBounds? = null,points: Int = 8192): PhotoTrackGeometry =
        throw UnsupportedOperationException("Track geometry unavailable")
    suspend fun route(query: PhotoQuery,bounds: PhotoMapBounds? = null,points: Int = 4096): PhotoRouteData =
        throw UnsupportedOperationException("Photo route unavailable")
    fun thumbnail(photo: Photo, size: Int): String
    fun original(photo: Photo): String
    suspend fun download(photo: Photo, destination: () -> OutputStream, progress: (Long,Long) -> Unit): Long =
        throw UnsupportedOperationException("Download unavailable")
}

class PhotosApi(private val client: OkHttpClient, address: String) : PhotosService {
    internal val streamingClient=client.newBuilder().callTimeout(0,TimeUnit.MILLISECONDS).build()
    private val base = Connections.address(address).resolve("api/photos/v1/")!!
    private fun url(path: String, query: Map<String,String>) = base.resolve(path)!!.newBuilder()
        .apply { query.forEach { (key,value) -> addQueryParameter(key,value) } }.build()
    private suspend fun json(path: String, query: Map<String,String> = emptyMap(), maxBytes: Long = 2 * 1024 * 1024L) =
        client.json(Request.Builder().url(url(path,query)).build(),maxBytes) { status, code ->
            when {
                status == 401 -> UiText(R.string.error_auth)
                status == 403 -> UiText(R.string.error_photos_permission)
                code == "search_too_broad" -> UiText(R.string.error_search_broad)
                status == 404 -> UiText(R.string.error_missing)
                else -> UiText(R.string.error_photos_server,status)
            }
        }
    override suspend fun session(): PhotoSession {
        val o = json("session",maxBytes=64 * 1024)
        requireMessage(o.getInt("protocol") == 1,R.string.error_photos_protocol)
        val scope = JSONObject().put("instance",o.getString("instance")).put("dataset",o.getString("dataset"))
            .put("account",o.getString("account")).toString()
        val settings = o.getJSONObject("settings")
        return PhotoSession(scope,o.getBoolean("can_manage_people"),settings.getInt("thumbnail_size").coerceIn(80,640),
            settings.getInt("folder_thumbnail_size").coerceIn(80,640),settings.getInt("preview_size").coerceIn(640,2048),
            settings.getInt("large_preview_size").coerceIn(640,4096),settings.getInt("slideshow_seconds").coerceIn(3,300),
            settings.getInt("frame_seconds").coerceIn(3,300))
    }
    override suspend fun browse(query: PhotoQuery, page: Int, section: String): PhotoPage {
        val o = json("browse",mapOf("path" to query.path,"q" to query.query,"page" to "$page","section" to section,
            "recursive" to if(query.recursive) "1" else "0","gps" to if(query.gps) "1" else "0",
            "sort" to query.sort,"type" to query.type))
        return PhotoPage(o.getString("path"),o.getString("parent"),o.getInt("page"),o.getInt("total"),o.getBoolean("has_next"),
            o.getInt("folder_total"),o.getBoolean("folder_has_next"),o.getBoolean("blog_has_next"),
            o.getJSONArray("media").objects(::photo),o.getJSONArray("folders").objects { folder ->
                PhotoFolder(folder.getString("path"),folder.getString("name"),folder.optionalString("date"),
                    folder.getInt("media_count"),folder.getBoolean("approximate"),folder.getInt("folder_count"),
                    folder.getJSONArray("previews").objects(::photo))
            },o.getJSONArray("blogs").objects(::post))
    }
    override suspend fun info(path: String) = photo(json("media/info",mapOf("path" to path)).getJSONObject("media"))
    override suspend fun locateDate(date: String): PhotoDatePosition {
        val o = try { json("browse/date", mapOf("date" to date), maxBytes = 64 * 1024) }
        catch(e: ApiFailure) {
            if(e.status == 404) throw UserIoFailure(UiText(R.string.photos_date_unavailable))
            throw e
        }
        val result = PhotoDatePosition(o.getString("path"), o.getString("date"), o.getInt("page"))
        requireMessage(result.page in 1..1_000_000 &&
            if(result.path.isEmpty()) result.date.isEmpty() && result.page == 1
            else runCatching { java.time.LocalDate.parse(result.date) }.isSuccess,
            R.string.error_response_invalid)
        return result
    }
    override suspend fun map(query: PhotoQuery, bounds: PhotoMapBounds?): PhotoMapData {
        val o=json("map",mapParams(query,bounds))
        val b=o.optJSONObject("bounds")
        return PhotoMapData(o.getInt("total"),b?.let(::mapBounds),
            o.getJSONArray("markers").objects {PhotoMapMarker(it.getDouble("latitude"),it.getDouble("longitude"),it.getInt("count"),it.optString("path"),it.optJSONObject("bounds")?.let(::mapBounds))})
    }
    override suspend fun mapMedia(query: PhotoQuery,bounds: PhotoMapBounds,page: Int): PhotoMapPage {
        val o=json("map/media",mapParams(query,bounds)+("page" to "$page"))
        return PhotoMapPage(o.getInt("total"),o.getInt("page"),o.getBoolean("has_next"),o.getJSONArray("media").objects(::photo))
    }
    override suspend fun tracks(path: String,cursor: String,before: Boolean): PhotoTrackPage {
        val o=json("map/tracks",mapOf("path" to path,"cursor" to cursor,"before" to if(before) "1" else "0"))
        val files=o.getJSONArray("tracks")
        requireMessage(files.length()<=32,R.string.error_response_invalid)
        return PhotoTrackPage(files.objects {
            val file=PhotoTrackFile(it.getString("path"),it.getString("name"),it.getString("modified"),it.getLong("bytes"))
            requireMessage(file.path.length<=4096 && file.name.length<=1024 && file.modified.length<=128 && file.bytes>=0,R.string.error_response_invalid)
            file
        },
            o.getString("cursor"),o.getString("previous_cursor"),o.getBoolean("has_next"),o.getBoolean("has_previous"),o.getBoolean("ready"))
    }
    override suspend fun track(path: String,bounds: PhotoMapBounds?,points: Int): PhotoTrackGeometry {
        require(points in 32..8192)
        val o=json("map/track",mapParams(PhotoQuery(path=path),bounds)+("points" to "$points"))
        return trackGeometry(o,points)
    }
    override suspend fun route(query: PhotoQuery,bounds: PhotoMapBounds?,points: Int): PhotoRouteData {
        require(points in 32..8192)
        val o=json("map/route",mapParams(query,bounds)+("points" to "$points"))
        val total=o.getInt("total_media");val radius=o.getInt("radius_meters")
        requireMessage(total>=0 && radius in 500..10000,R.string.error_response_invalid)
        return PhotoRouteData(trackGeometry(o,points),total,radius)
    }
    private fun trackGeometry(o: JSONObject,points: Int): PhotoTrackGeometry {
        val lines=o.getJSONArray("segments")
        var count=0
        requireMessage(lines.length()<=points,R.string.error_response_invalid)
        val segments=List(lines.length()) {i ->
            val line=lines.getJSONArray(i)
            count+=line.length()
            requireMessage(count<=points,R.string.error_response_invalid)
            List(line.length()) {j ->
                val point=line.getJSONArray(j)
                val lat=point.getDouble(0);val lon=point.getDouble(1)
                requireMessage(point.length()==2 && lat.isFinite() && lon.isFinite() && lat in -90.0..90.0 && lon in -180.0..180.0,R.string.error_response_invalid)
                PhotoMapPoint(lat,lon)
            }
        }
        return PhotoTrackGeometry(o.getString("path"),o.getString("name"),o.optJSONObject("bounds")?.let(::mapBounds),segments,
            o.getInt("total_points"),o.getBoolean("simplified"),o.getInt("omitted_segments"))
    }
    private fun mapBounds(o: JSONObject)=PhotoMapBounds(o.getDouble("south"),o.getDouble("west"),o.getDouble("north"),o.getDouble("east"))
    private fun mapParams(query: PhotoQuery,bounds: PhotoMapBounds?)=mutableMapOf("path" to query.path,"q" to query.query,"type" to query.type).apply {
        bounds?.let { putAll(mapOf("south" to "${it.south}","west" to "${it.west}","north" to "${it.north}","east" to "${it.east}")) }
    }
    // One MiB of source text can expand through HTML and JSON escaping.
    override suspend fun blog(path: String) = post(json("blog",mapOf("path" to path),20 * 1024 * 1024L).getJSONObject("blog"))
    override fun thumbnail(photo: Photo, size: Int) = url("thumbnail",mapOf("path" to photo.path,"size" to "$size","v" to photo.version)).toString()
    override fun original(photo: Photo) = url("media",mapOf("path" to photo.path,"v" to photo.version)).toString()
    override suspend fun download(photo: Photo, destination: () -> OutputStream, progress: (Long,Long) -> Unit): Long {
        // Shares TLS policy, authentication, pool and dispatcher. Large originals
        // have no total call deadline, but retain connection/read timeouts.
        return streamingClient.readResponse(Request.Builder().url(original(photo)).build(),waitForClose=true) { response, active ->
            if(response.code!=200) throw ApiFailure(response.code,"download_failed",UiText(R.string.error_download_status,response.code))
            val body=response.body ?: throw UserIoFailure(UiText(R.string.error_response_empty))
            val mime=body.contentType()?.let {"${it.type}/${it.subtype}"}
            if(mime!=photo.mime && mime!="application/octet-stream") throw UserIoFailure(UiText(R.string.error_download_mime))
            if(!active()) throw UserIoFailure(UiText(R.string.error_download_cancelled))
            val total=body.contentLength()
            var bytes=0L
            var lastUpdate=0L
            destination().use { output ->
                body.byteStream().use { input ->
                    val buffer=ByteArray(64 * 1024)
                    while(active()) {
                        val count=input.read(buffer)
                        if(count<0) break
                        output.write(buffer,0,count)
                        bytes+=count
                        val now=System.nanoTime()
                        if(now-lastUpdate>100_000_000L) {progress(bytes,total);lastUpdate=now}
                    }
                }
                if(!active()) throw UserIoFailure(UiText(R.string.error_download_cancelled))
                if(total>=0 && bytes!=total) throw UserIoFailure(UiText(R.string.error_download_incomplete))
                output.flush()
            }
            progress(bytes,total)
            bytes
        }
    }
    private fun photo(o: JSONObject) = Photo(o.getString("path"),o.getString("name"),o.getString("type"),o.getString("mime"),
        o.getString("version"),o.getString("modified"),o.optionalString("captured"),o.getLong("bytes"),o.getInt("width"),o.getInt("height"),
        o.optString("camera"),o.optString("lens"),o.optionalDouble("latitude"),o.optionalDouble("longitude"),
        o.optionalDouble("rating"),o.stringList("tags"),o.stringList("keywords"),
        ((o.optJSONArray("faces")?.objects {it.optString("Name")} ?: emptyList()) +
            (o.optJSONArray("automatic_faces")?.objects {it.optString("name")} ?: emptyList())).distinct())
    private fun post(o: JSONObject) = PhotoBlog(o.getString("path"),o.getString("name"),o.optionalString("date"),o.getString("modified"),o.optString("text"),o.optString("html"))
}
private fun JSONObject.optionalString(key: String): String? = if(isNull(key)) null else optString(key).takeIf { it.isNotBlank() }
private fun JSONObject.optionalDouble(key: String): Double? = if(isNull(key)) null else optDouble(key).takeIf { it.isFinite() }
private fun JSONObject.stringList(key: String): List<String> = optJSONArray(key)?.let {array ->
    List(array.length()) {array.getString(it)}.filter(String::isNotBlank).distinct()
} ?: emptyList()
private fun <T> JSONArray.objects(convert: (JSONObject) -> T): List<T> = List(length()) { convert(getJSONObject(it)) }
