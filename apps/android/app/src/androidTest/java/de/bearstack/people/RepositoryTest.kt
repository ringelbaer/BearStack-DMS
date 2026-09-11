package de.bearstack.people

import androidx.room.Room
import androidx.test.platform.app.InstrumentationRegistry
import de.bearstack.people.data.local.*
import de.bearstack.people.data.remote.*
import de.bearstack.people.people.*
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.runBlocking
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test
import java.io.IOException

internal class FakeService : LabelingService {
    var upper=2L
    var supportsMerges=true
    var supportsMergeNaming=true
    var supportsMergeSideActions=true
    val session get() = Session("instance","dataset","account",upper,namedPeople=true,namedSearch=true,mergeSuggestions=supportsMerges,mergeNaming=supportsMergeNaming,manualMerge=true,mergeSideActions=supportsMergeSideActions)
    val mergePairs=mutableListOf<MergeSuggestion>()
    var failNextMerge=false
    override suspend fun nextMergeSuggestion(excluded: Pair<Long,Long>?): MergeSuggestion? {
        if(failNextMerge) throw IOException("Nächster Vorschlag nicht erreichbar")
        return mergePairs.firstOrNull {excluded==null || setOf(it.source.id,it.target.id)!=setOf(excluded.first,excluded.second)}
    }
    var actionDelay=0L
    val people = mutableMapOf(1L to Person(1,"",1,5,10,listOf(10,11,12,13)),2L to Person(2,"",1,1,20,listOf(20)))
    val receipts = mutableMapOf<String,Receipt>()
    var commits = 0
    var loseResponse = false
    var failPerson: Long? = null
    override suspend fun session() = session
    override suspend fun candidates(after: Long,upper: Long) = Candidates(people.values.filter { it.id>after && it.id<=upper && it.name.isEmpty() },upper,false)
    val groupRequests=mutableListOf<Triple<Long,Long,Boolean>>()
    override suspend fun mergeGroups(after:Long,upper:Long,includeNamed:Boolean):Candidates {
        groupRequests+=Triple(after,upper,includeNamed)
        val groups=people.values.filter {it.id>after && it.id<=upper && (includeNamed || it.name.isEmpty())}.sortedBy {it.id}.take(21)
        val page=groups.take(20).map {it.copy(faces=listOf(it.faceId))}
        return Candidates(page,page.lastOrNull()?.id ?: after,groups.size>20)
    }
    override suspend fun namedPeople(after: Long,upper: Long): Candidates {
        val page=people.values.filter {it.id>after && it.id<=upper && it.name.isNotEmpty()}.sortedBy {it.id}.take(21)
        return Candidates(page.take(20),page.take(20).lastOrNull()?.id ?: after,page.size>20)
    }
    val directoryQueries=mutableListOf<String>()
    var directoryDelay=0L
    override suspend fun searchPeople(after: Long,upper: Long,q: String): Candidates {
        directoryQueries+=q
        if(directoryDelay>0) kotlinx.coroutines.delay(directoryDelay)
        val page=people.values.filter {it.id>after && it.id<=upper && it.name.isNotEmpty() && it.name.contains(q,true)}.sortedBy {it.id}.take(21)
        return Candidates(page.take(20),page.take(20).lastOrNull()?.id ?: after,page.size>20)
    }
    override suspend fun personFaces(id: Long,offset: Int,after: Long): Person {
        val p=person(id,0)
        val all=if(p.count==5L && p.faces==listOf(10L,11L,12L,13L)) p.faces+14L else p.faces
        return p.copy(offset=0,faces=all.filter {it>after}.take(40))
    }
    override suspend fun person(id: Long,offset: Int): Person {
        if (failPerson == id) throw IOException("next group unavailable")
        val person=people[id] ?: throw ApiFailure(404,"not_found","gone")
        return if(offset==4 && person.count==5L) person.copy(offset=4,faces=listOf(14)) else person
    }
    var matches: List<FaceMatch> = emptyList()
    var matchUpdates: List<List<FaceMatch>> = emptyList()
    var matchFinish: kotlinx.coroutines.CompletableDeferred<Unit>? = null
    var matchDelay=0L
    var matchFailure=false
    var cancelledMatches=0
    val matchedFaces=mutableListOf<Long>()
    override fun faceMatches(face: Long) = flow {
        matchedFaces+=face
        try {
            matchUpdates.forEach { emit(it) }
            matchFinish?.await() ?: kotlinx.coroutines.delay(matchDelay)
        }
        catch(e: kotlinx.coroutines.CancellationException) { cancelledMatches++;throw e }
        if(matchFailure) throw IOException("face search unavailable")
        emit(matches)
    }
    val queries = mutableListOf<String>()
    var slowQuery: String? = null
    var cancelledQueries = 0
    override suspend fun suggestions(q: String,exact: Boolean): List<Person> {
        queries += q
        if (q == slowQuery) try { kotlinx.coroutines.delay(2000) } catch(e:kotlinx.coroutines.CancellationException) { cancelledQueries++;throw e }
        return people.values.filter { it.name.contains(q) && it.name.isNotEmpty() }
    }
    override suspend fun receipt(operation: String,dataset: String) = receipts[operation] ?: throw ApiFailure(404,"not_found","no receipt")
    override suspend fun action(id: Long,body: String): Receipt {
        if(actionDelay>0) kotlinx.coroutines.delay(actionDelay)
        val request=JSONObject(body); val op=request.getString("operation_id")
        receipts[op]?.let {return it}
        val p=people[id] ?: throw ApiFailure(409,"conflict","gone")
        if(p.revision!=request.getLong("revision")) throw ApiFailure(409,"conflict","stale")
        val action=request.getString("action")
        if(action=="merge_groups" || action=="name_groups") {
            val refs=request.getJSONArray("groups")
            val selected=(0 until refs.length()).map {i ->
                val ref=refs.getJSONObject(i)
                val person=people[ref.getLong("id")] ?: throw ApiFailure(409,"conflict","gone")
                if(person.revision!=ref.getLong("revision")) throw ApiFailure(409,"conflict","stale")
                person
            }
            val name=if(action=="name_groups") request.getString("name") else selected.firstOrNull {it.name.isNotEmpty()}?.name.orEmpty()
            if(action=="name_groups" && !request.optBoolean("allow_duplicate") && people.values.any {it.name==name && it !in selected})
                throw ApiFailure(409,"name_exists","duplicate")
            people[id]=p.copy(name=name,count=selected.sumOf {it.count},faces=selected.flatMap {it.faces},revision=p.revision+1)
            selected.drop(1).forEach {people.remove(it.id)}
            val receipt=Receipt(op,action,id,id,0,selected.sumOf {it.count},if(action=="name_groups") selected.size else 0,100,p.revision+1)
            commits++;receipts[op]=receipt
            if(loseResponse) {loseResponse=false;throw IOException("response lost after commit")}
            return receipt
        }
        if(action=="accept_merge" || action=="reject_merge" || action=="name_merge") {
            val pair=mergePairs.firstOrNull {it.id==request.getLong("suggestion_id")} ?: throw ApiFailure(409,"conflict","gone")
            val target=people[request.getLong("target_id")] ?: throw ApiFailure(409,"conflict","gone")
            if(pair.source.id!=id || pair.target.id!=target.id || target.revision!=request.getLong("target_revision")) throw ApiFailure(409,"conflict","stale")
            if(action=="name_merge") {
                val assigned=request.optLong("assign_id")
                val destination=if(assigned==0L) target.copy(name=request.getString("name")) else people.getValue(assigned)
                people[destination.id]=destination.copy(count=p.count+target.count+(if(assigned==0L)0 else destination.count),revision=destination.revision+1)
                people.remove(id)
                if(assigned!=0L) people.remove(target.id)
                mergePairs.remove(pair)
                val receipt=Receipt(op,action,id,destination.id,0,p.count+target.count,2,100)
                commits++;receipts[op]=receipt
                if(loseResponse) {loseResponse=false;throw IOException("response lost after commit")}
                return receipt
            }
            if(action=="accept_merge") {
                people[target.id]=target.copy(count=target.count+p.count,faces=target.faces+p.faces,revision=target.revision+1)
                people.remove(id)
                mergePairs.removeAll {it.source.id in listOf(id,target.id) || it.target.id in listOf(id,target.id)}
            } else mergePairs.remove(pair)
            val receipt=Receipt(op,action,id,target.id,0,if(action=="accept_merge")p.count else 0,0,100,if(action=="accept_merge")0 else p.revision)
            commits++;receipts[op]=receipt
            if(loseResponse) {loseResponse=false;throw IOException("response lost after commit")}
            return receipt
        }
        if(action in listOf("rename","favorite","unassign")) {
            val face=request.optLong("face_id")
            val newId=if(action=="unassign") (people.keys.maxOrNull() ?: 0)+1 else 0L
            when(action) {
                "rename" -> people[id]=p.copy(name=request.getString("name"),revision=p.revision+1)
                "favorite" -> people[id]=p.copy(favorites=if(request.getBoolean("favorite")) p.favorites+face else p.favorites-face,revision=p.revision+1)
                "unassign" -> {
                    people[newId]=Person(newId,"",1,1,face,listOf(face))
                    if(p.count==1L) people.remove(id)
                    else people[id]=p.copy(count=p.count-1,revision=p.revision+1,faces=p.faces.filterNot {it==face},favorites=p.favorites-face)
                }
            }
            val receipt=Receipt(op,action,id,0,newId,if(action=="rename")p.count else 1,0,100,people[id]?.revision ?: p.revision+1)
            commits++;receipts[op]=receipt
            if(loseResponse) {loseResponse=false;throw IOException("response lost after commit")}
            return receipt
        }
        val target=if(action=="assign") people[request.getLong("target_id")] ?: throw ApiFailure(409,"conflict","gone") else null
        if(target!=null && (target.name.isEmpty() || target.id==id || target.revision!=request.getLong("target_revision"))) throw ApiFailure(409,"conflict","stale")
        val receipt=Receipt(op,action,id,target?.id ?: 0,if(action=="detach") 3 else 0,if(action=="detach")1 else p.count,if(action=="detach")0 else 1,100)
        if(action=="detach") {
            people[3]=Person(3,"",1,1,request.getLong("face_id"),listOf(request.getLong("face_id")))
            people[id]=p.copy(count=p.count-1,revision=p.revision+1,faces=p.faces.filterNot { it==request.getLong("face_id") }+14L)
        } else if(action=="name") people[id]=p.copy(name=request.getString("name"),revision=p.revision+1)
        else {
            if(target!=null) people[target.id]=target.copy(count=target.count+p.count,faces=target.faces+p.faces,revision=target.revision+1)
            people.remove(id)
        }
        // Like the server, mutations invalidate cached suggestions for this group.
        mergePairs.removeAll {it.source.id==id || it.target.id==id}
        commits++; receipts[op]=receipt
        if(loseResponse) {loseResponse=false;throw IOException("response lost after commit")}
        return receipt
    }
}
class RepositoryTest {
    private fun database() = Room.inMemoryDatabaseBuilder(InstrumentationRegistry.getInstrumentation().targetContext,LabelingDatabase::class.java).build()
    @Test fun manualMergePersistsWholeSelectionAndRecoversReceiptAfterRestart()=runBlocking {
        val db=database()
        try {
            val api=FakeService()
            val repo=PeopleRepository(db,api,api.session)
            repo.next();repo.skip(api.people.getValue(1));repo.next()
            repo.prepareGroupMerge(listOf(api.people.getValue(1),api.people.getValue(2)))
            val body=JSONObject(repo.pending()!!.body)
            assertEquals(2,body.getJSONArray("groups").length())
            api.loseResponse=true
            try {repo.resolve();fail("expected lost response")} catch(_:IOException) {}
            assertNotNull(repo.pending());assertEquals(1,api.commits)
            val restored=PeopleRepository(db,api,api.session)
            assertNotNull(restored.resolve());assertNull(restored.pending());assertEquals(1,api.commits)
            assertEquals("",restored.state().skipped)
            assertEquals(1L,restored.next()!!.id)
        } finally {db.close()}
    }
    @Test fun mergeDecisionSurvivesRestartAndRejectPreservesCurrentPage() = runBlocking {
        for(accept in listOf(false,true)) {
            val db=database()
            try {
                val api=FakeService()
                val repo=PeopleRepository(db,api,api.session)
                repo.next();repo.page(4)
                val source=api.people.getValue(1)
                val target=api.people.getValue(2)
                api.mergePairs+=MergeSuggestion(7,source,target)
                repo.prepare(source,if(accept) "accept_merge" else "reject_merge",target=target,suggestionId=7)
                val saved=JSONObject(repo.pending()!!.body)
                assertEquals(7L,saved.getLong("suggestion_id"));assertEquals(target.revision,saved.getLong("target_revision"))
                api.loseResponse=true
                try {repo.resolve();fail("expected lost response")} catch(_:IOException) {}
                val restored=PeopleRepository(db,api,api.session)
                assertNotNull(restored.resolve());assertNull(restored.pending());assertEquals(1,api.commits)
                if(accept) assertEquals(2L,restored.next()!!.id)
                else {assertEquals(1L,restored.next()!!.id);assertEquals(4,restored.state().page)}
                assertTrue(restored.statistics(0).first().none {it.action=="name" || it.action=="assign"})
            } finally {db.close()}
        }
    }
    @Test fun unsentIgnoresRecoverAfterRestartAndReceiptsKeepCurrentCard() = runBlocking {
        val db=database()
        try {
            val api=FakeService();var repo=PeopleRepository(db,api,api.session)
            val ignored=repo.next()!!;repo.stageIgnore(ignored)
            assertEquals(2L,repo.next()!!.id)
            repo=PeopleRepository(db,api,api.session)
            repo.restoreIgnores()
            assertEquals("",repo.state().stagedIgnores);assertEquals(0,api.commits)
            repo.skip(repo.next()!!);assertEquals(1L,repo.next()!!.id)
            repo.stageIgnore(repo.next()!!)
            repo.newPass(false) // A new pass must not reoffer an ignore still awaiting confirmation.
            assertNull(repo.next())
            repo.prepare(ignored,"ignore");repo.resolve()
            assertEquals("",repo.state().stagedIgnores)
            assertEquals(5L,repo.statistics(0).first().single {it.action=="ignore"}.faces)
        } finally {db.close()}
    }
    @Test fun backRestoresPageAndCurrentCardAfterRecreationAndCorrectsStatistics() = runBlocking {
        val db=database()
        try {
            val api=FakeService();var repo=PeopleRepository(db,api,api.session)
            repo.next();repo.skip(repo.page(4));assertEquals(2L,repo.next()!!.id)
            assertEquals(5L,repo.statistics(0).first().single().faces)
            repo=PeopleRepository(db,api,api.session)
            val restored=repo.back()!!
            assertEquals(1L,restored.id);assertEquals(4,restored.offset)
            assertEquals("",repo.state().skipped);assertTrue(repo.statistics(0).first().isEmpty())
            repo.prepare(restored,"name",name="Anna");repo.resolve()
            assertEquals(2L,repo.next()!!.id);assertNull(repo.back())
            repo.skip(repo.next()!!);assertNull(repo.next())
            assertEquals(2L,repo.back()!!.id) // Also works from the end of a pass.
            repo.skip(repo.next()!!)
            assertEquals(1L,repo.statistics(0).first().single {it.action=="skip"}.groups)
        } finally {db.close()}
    }
    @Test fun repeatedBackPreservesResumeOrderAndNewPassClearsHistory() = runBlocking {
        val db=database()
        try {
            val api=FakeService();api.people[3]=Person(3,"",1,1,30,listOf(30))
            val repo=PeopleRepository(db,api,api.session.copy(upper=3))
            repo.skip(repo.next()!!);repo.skip(repo.next()!!);assertEquals(3L,repo.next()!!.id)
            assertEquals(2L,repo.back()!!.id);assertEquals(1L,repo.back()!!.id)
            assertTrue(repo.statistics(0).first().isEmpty())
            repo.prepare(repo.next()!!,"name",name="Anna");repo.resolve()
            assertEquals(2L,repo.next()!!.id)
            repo.prepare(repo.next()!!,"name",name="Ben");repo.resolve()
            assertEquals(3L,repo.next()!!.id)
            repo.skip(repo.next()!!);assertNull(repo.next())
            repo.newPass(true);assertNull(repo.back());assertEquals(3L,repo.next()!!.id)
        } finally {db.close()}
    }
    @Test fun backFailurePreservesQueueAndUnavailableGroupsAreNotRestored() = runBlocking {
        val db=database()
        try {
            val api=FakeService();val repo=PeopleRepository(db,api,api.session)
            repo.skip(repo.next()!!);val current=repo.next()!!
            val before=repo.state();api.failPerson=1
            try {repo.back();fail("network failure expected")}catch(_:IOException){}
            assertEquals(before,repo.state());assertEquals(1L,repo.statistics(0).first().single().groups)
            api.failPerson=null
            repo.prepare(current,"name",name="Ben")
            try {repo.back();fail("pending action must block back")}catch(_:IllegalStateException){}
            assertEquals(before,repo.state());db.dao().clearPending(repo.scope)
            api.people[1]=api.people[1]!!.copy(name="Extern benannt",revision=2)
            assertNull(repo.back());assertEquals(current.id,repo.state().current)
            assertEquals("",repo.state().skipHistory)
            val other=PeopleRepository(db,api,api.session.copy(account="other"))
            assertNull(other.back());assertTrue(other.statistics(0).first().isEmpty())
        } finally {db.close()}
    }
    @Test fun lostCommitResponseResolvedAfterRepositoryRecreationWithoutDoubleCount() = runBlocking {
        val db=database()
        try {
            val api=FakeService();var repo=PeopleRepository(db,api,api.session)
            val p=repo.next()!!
            assertEquals(listOf(14L),repo.page(4).faces)
            repo.prepare(p,"name",name="Anna");api.loseResponse=true
            try {repo.resolve();fail("expected lost response")} catch(_: IOException) {}
            assertNotNull(repo.pending())
            repo=PeopleRepository(db,api,api.session)
            assertNotNull(repo.resolve());assertNull(repo.resolve())
            assertEquals(1,api.commits)
            val stats=repo.statistics(0).first().single()
            assertEquals(5L,stats.faces);assertEquals(1L,stats.groups)
            assertEquals(2L,repo.next()!!.id)
        } finally {db.close()}
    }
    @Test fun unsentPendingResendsSameIdAndStaleDecisionDoesNotRetryAutomatically() = runBlocking {
        val db=database()
        try {
            val api=FakeService();val repo=PeopleRepository(db,api,api.session)
            val p=repo.next()!!;repo.prepare(p,"ignore")
            val saved=repo.pending()!!
            api.people[1]=p.copy(revision=2)
            try {repo.resolve();fail("expected conflict")} catch(e: ApiFailure){assertEquals(409,e.status)}
            assertNull(repo.pending());assertEquals(0,api.commits)
            assertEquals(2L,repo.next()!!.revision)
            repo.prepare(repo.next()!!,"ignore")
            assertNotEquals(saved.operation,repo.pending()!!.operation)
            repo.resolve();assertEquals(1,api.commits)
        } finally {db.close()}
    }
    @Test fun detachPrioritySkipPassScopeIsolationAndOncePerPassStatistics() = runBlocking {
        val db=database()
        try {
            val api=FakeService();val repo=PeopleRepository(db,api,api.session)
            repo.prepare(repo.next()!!,"detach",face=10);repo.resolve()
            assertEquals(1L,repo.next()!!.id)
            repo.prepare(repo.next()!!,"name",name="Anna");repo.resolve()
            assertEquals(3L,repo.next()!!.id)
            repo.skip(repo.next()!!);assertEquals(2L,repo.next()!!.id)
            repo.skip(repo.next()!!);assertNull(repo.next())
            repo.newPass(true);assertEquals(3L,repo.next()!!.id)
            assertEquals(2L,repo.statistics(0).first().first { it.action=="skip" }.groups)
            val other=PeopleRepository(db,api,api.session.copy(account="other"))
            assertTrue(other.statistics(0).first().isEmpty());assertEquals("",other.state().skipped)
        } finally {db.close()}
    }
}
