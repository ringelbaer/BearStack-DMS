package de.bearstack.people

import android.database.sqlite.SQLiteDatabase
import androidx.room.Room
import androidx.test.platform.app.InstrumentationRegistry
import de.bearstack.people.data.local.LabelingDatabase
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test
import java.util.UUID

class DatabaseMigrationTest {
    @Test fun versionOneKeepsQueuePendingActionAndStatistics() = runBlocking {
        val instrumentation=InstrumentationRegistry.getInstrumentation()
        val context=instrumentation.targetContext
        val name="migration-${UUID.randomUUID()}.db"
        val file=context.getDatabasePath(name)
        file.parentFile!!.mkdirs()
        val schema=instrumentation.context.assets.open("de.bearstack.people.data.local.LabelingDatabase/1.json")
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
                old.execSQL("INSERT INTO queue_state VALUES ('scope',100,'pass',2,4,20,0,'3,4','101','1')")
                old.execSQL("INSERT INTO events VALUES ('scope','skip:pass:1','skip',5,1,100)")
                old.execSQL("INSERT INTO pending VALUES ('scope','operation',2,'{}')")
                old.version=1
            }
            val migrated=Room.databaseBuilder(context,LabelingDatabase::class.java,name)
                .addMigrations(LabelingDatabase.MIGRATION_1_2,LabelingDatabase.MIGRATION_2_3).build()
            try {
                val state=migrated.dao().state("scope")!!
                assertEquals(2L,state.current);assertEquals(4,state.page)
                assertEquals("3,4",state.remaining);assertEquals("101",state.detached);assertEquals("1",state.skipped)
                assertEquals("",state.skipHistory);assertEquals("",state.resume);assertEquals("",state.stagedIgnores)
                assertEquals("operation",migrated.dao().pending("scope")!!.operation)
                assertEquals(5L,migrated.dao().statistics("scope",0).first().single().faces)
            } finally {migrated.close()}
        } finally {context.deleteDatabase(name)}
    }
}
