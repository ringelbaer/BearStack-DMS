package de.bearstack.people

import android.database.sqlite.SQLiteDatabase
import androidx.room.Room
import androidx.test.platform.app.InstrumentationRegistry
import de.bearstack.people.data.local.*
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test
import java.util.UUID

class DatabaseMigrationTest {
    private fun migrate(version: Int, seed: (SQLiteDatabase) -> Unit, verify: suspend (LabelingDatabase) -> Unit) = runBlocking {
        val instrumentation=InstrumentationRegistry.getInstrumentation()
        val context=instrumentation.targetContext
        val name="migration-${UUID.randomUUID()}.db"
        val file=context.getDatabasePath(name)
        file.parentFile!!.mkdirs()
        val schema=instrumentation.context.assets.open("de.bearstack.people.data.local.LabelingDatabase/$version.json")
            .bufferedReader().use {JSONObject(it.readText()).getJSONObject("database")}
        try {
            SQLiteDatabase.openOrCreateDatabase(file,null).use { old ->
                val entities=schema.getJSONArray("entities")
                for(i in 0 until entities.length()) {
                    val entity=entities.getJSONObject(i)
                    val table=entity.getString("tableName")
                    old.execSQL(entity.getString("createSql").replace("\${TABLE_NAME}",table))
                    val indices=entity.optJSONArray("indices")
                    if(indices!=null) for(j in 0 until indices.length()) old.execSQL(indices.getJSONObject(j).getString("createSql").replace("\${TABLE_NAME}",table))
                }
                seed(old)
                old.version=version
            }
            // Opening through Room also verifies every migrated column and index.
            val migrated=Room.databaseBuilder(context,LabelingDatabase::class.java,name)
                .addMigrations(LabelingDatabase.MIGRATION_1_2,LabelingDatabase.MIGRATION_2_3,LabelingDatabase.MIGRATION_3_4).build()
            try {verify(migrated)} finally {migrated.close()}
        } finally {context.deleteDatabase(name)}
    }

    @Test fun versionOneKeepsQueuePendingActionAndStatistics() = migrate(1,{old ->
        old.execSQL("INSERT INTO queue_state VALUES ('scope',100,'pass',2,4,20,0,'3,4','101','1')")
        old.execSQL("INSERT INTO events VALUES ('scope','skip:pass:1','skip',5,1,100)")
        old.execSQL("INSERT INTO pending VALUES ('scope','operation',2,'{}')")
    }) {db ->
        val dao=db.dao();val state=dao.state("scope")!!
        assertEquals(2L,state.current);assertEquals(4,state.page);assertEquals(0L,state.revision)
        assertEquals(3L,dao.firstEntry("scope",QueueKind.Remaining)!!.person)
        assertEquals(4L,dao.lastEntry("scope",QueueKind.Remaining)!!.person)
        assertEquals(101L,dao.firstEntry("scope",QueueKind.Detached)!!.person)
        assertEquals(1,dao.queueStatus("scope").skipped)
        assertFalse(dao.queueStatus("scope").canGoBack)
        assertEquals(0,dao.entryCount("scope",QueueKind.Resume))
        assertEquals(0,dao.entryCount("scope",QueueKind.Staged))
        assertEquals("operation",dao.pending("scope")!!.operation)
        assertEquals(5L,dao.statistics("scope",0).first().single().faces)
    }

    @Test fun versionThreeKeepsLargeQueuesPositionsAndAccountIsolation() = migrate(3,{old ->
        old.execSQL("INSERT INTO queue_state VALUES ('scope',20000,'pass',2,4,20,1,?,'101','1,5,1','1:8,5:12','8:4,9:0','10:12,11:4')",
            arrayOf((1..10000).joinToString(",")))
        old.execSQL("INSERT INTO queue_state VALUES ('other',7,'other-pass',0,0,0,0,'9,8','','','','','')")
        old.execSQL("INSERT INTO pending VALUES ('scope','pending-ignore',10,'{\"action\":\"ignore\"}')")
        old.execSQL("INSERT INTO events VALUES ('scope','receipt','name',12,2,100)")
    }) {db ->
        val dao=db.dao()
        assertEquals(10000,dao.entryCount("scope",QueueKind.Remaining))
        assertEquals(1L,dao.firstEntry("scope",QueueKind.Remaining)!!.person)
        assertEquals(10000L,dao.lastEntry("scope",QueueKind.Remaining)!!.person)
        assertEquals(9999L,dao.lastEntry("scope",QueueKind.Remaining)!!.position)
        assertEquals(2,dao.entryCount("scope",QueueKind.Skipped))
        assertEquals(8,dao.firstEntry("scope",QueueKind.History)!!.page)
        assertEquals(12,dao.lastEntry("scope",QueueKind.History)!!.page)
        assertEquals(8L,dao.firstEntry("scope",QueueKind.Resume)!!.person)
        assertEquals(4,dao.firstEntry("scope",QueueKind.Resume)!!.page)
        assertEquals(10L,dao.firstEntry("scope",QueueKind.Staged)!!.person)
        assertEquals(12,dao.firstEntry("scope",QueueKind.Staged)!!.page)
        assertEquals(9L,dao.firstEntry("other",QueueKind.Remaining)!!.person)
        assertEquals(8L,dao.lastEntry("other",QueueKind.Remaining)!!.person)
        assertEquals(2,dao.entryCount("other",QueueKind.Remaining))
        assertEquals("pending-ignore",dao.pending("scope")!!.operation)
        assertEquals(12L,dao.statistics("scope",0).first().single().faces)
        assertEquals(0,dao.entryCount("other",QueueKind.Staged))
        assertNull(dao.pending("other"))
    }

    @Test fun emptyVersionThreeDatabaseMigrates() = migrate(3,{}) {db ->
        assertNull(db.dao().state("scope"))
        assertEquals(0,db.dao().entryCount("scope",QueueKind.Remaining))
    }
}
