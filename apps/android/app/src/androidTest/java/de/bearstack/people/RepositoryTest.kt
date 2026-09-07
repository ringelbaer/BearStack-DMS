package de.bearstack.people

import androidx.room.Room
import androidx.test.platform.app.InstrumentationRegistry
import de.bearstack.people.data.local.*
import de.bearstack.people.data.remote.*
import de.bearstack.people.people.*
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test
import java.io.IOException

internal class FakeService : LabelingService {
    val session = Session("instance","dataset","account",2)
    val people = mutableMapOf(1L to Person(1,"",1,5,10,listOf(10,11,12,13)),2L to Person(2,"",1,1,20,listOf(20)))
    val receipts = mutableMapOf<String,Receipt>()
    var commits = 0
    var loseResponse = false
    var failPerson: Long? = null
    override suspend fun session() = session
    override suspend fun candidates(after: Long,upper: Long) = Candidates(people.values.filter { it.id>after && it.id<=upper && it.name.isEmpty() },upper,false)
    override suspend fun person(id: Long,offset: Int): Person {
        if (failPerson == id) throw IOException("next group unavailable")
        val person=people[id] ?: throw ApiFailure(404,"not_found","gone")
        return if(offset==4 && person.count==5L) person.copy(offset=4,faces=listOf(14)) else person
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
        val request=JSONObject(body); val op=request.getString("operation_id")
        receipts[op]?.let {return it}
        val p=people[id] ?: throw ApiFailure(409,"conflict","gone")
        if(p.revision!=request.getLong("revision")) throw ApiFailure(409,"conflict","stale")
        val action=request.getString("action")
        val receipt=Receipt(op,action,id,0,if(action=="detach") 3 else 0,if(action=="detach")1 else p.count,if(action=="detach")0 else 1,100)
        if(action=="detach") {
            people[3]=Person(3,"",1,1,request.getLong("face_id"),listOf(request.getLong("face_id")))
            people[id]=p.copy(count=p.count-1,revision=p.revision+1,faces=p.faces.filterNot { it==request.getLong("face_id") }+14L)
        } else people.remove(id)
        commits++; receipts[op]=receipt
        if(loseResponse) {loseResponse=false;throw IOException("response lost after commit")}
        return receipt
    }
}
class RepositoryTest {
    private fun database() = Room.inMemoryDatabaseBuilder(InstrumentationRegistry.getInstrumentation().targetContext,LabelingDatabase::class.java).build()
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
