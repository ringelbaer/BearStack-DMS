package de.bearstack.people.data.local

import android.content.Context
import androidx.room.*
import androidx.room.migration.Migration
import androidx.sqlite.db.SupportSQLiteDatabase
import kotlinx.coroutines.flow.Flow

@Entity(tableName = "queue_state")
data class QueueState(@PrimaryKey val scope: String, val upper: Long, val pass: String,
    val current: Long = 0, val page: Int = 0, val cursor: Long = 0, val exhausted: Boolean = false,
    val remaining: String = "", val detached: String = "", val skipped: String = "",
    @ColumnInfo(defaultValue="''") val skipHistory: String = "",
    @ColumnInfo(defaultValue="''") val resume: String = "",
    @ColumnInfo(defaultValue="''") val stagedIgnores: String = "")
@Entity(tableName = "pending")
data class Pending(@PrimaryKey val scope: String, val operation: String, val source: Long, val body: String)
@Entity(tableName = "events", primaryKeys = ["scope", "operation"], indices = [Index(value=["scope", "at"])])
data class Event(val scope: String, val operation: String, val action: String, val faces: Long, val groups: Int, val at: Long)
@Dao
interface LabelingDao {
    @Query("SELECT * FROM queue_state WHERE scope=:scope") suspend fun state(scope: String): QueueState?
    @Insert(onConflict = OnConflictStrategy.REPLACE) suspend fun state(state: QueueState)
    @Query("SELECT * FROM pending WHERE scope=:scope") suspend fun pending(scope: String): Pending?
    @Insert(onConflict = OnConflictStrategy.ABORT) suspend fun pending(pending: Pending)
    @Query("DELETE FROM pending WHERE scope=:scope") suspend fun clearPending(scope: String)
    @Insert(onConflict = OnConflictStrategy.IGNORE) suspend fun event(event: Event): Long
    @Query("DELETE FROM events WHERE scope=:scope AND operation=:operation AND action='skip'")
    suspend fun undoSkipEvent(scope: String, operation: String)
    @Query("SELECT action, SUM(faces) AS faces, SUM(`groups`) AS `groups`, SUM(CASE WHEN at>=:today THEN faces ELSE 0 END) AS todayFaces, SUM(CASE WHEN at>=:today THEN `groups` ELSE 0 END) AS todayGroups FROM events WHERE scope=:scope GROUP BY action")
    fun statistics(scope: String, today: Long): Flow<List<Statistics>>
}
data class Statistics(val action: String, val faces: Long, val groups: Long, val todayFaces: Long, val todayGroups: Long)
@Database(entities=[QueueState::class, Pending::class, Event::class], version=3, exportSchema=true)
abstract class LabelingDatabase : RoomDatabase() {
    abstract fun dao(): LabelingDao
    companion object {
        val MIGRATION_2_3 = object : Migration(2,3) {
            override fun migrate(db: SupportSQLiteDatabase) {
                db.execSQL("ALTER TABLE queue_state ADD COLUMN stagedIgnores TEXT NOT NULL DEFAULT ''")
            }
        }
        val MIGRATION_1_2 = object : Migration(1,2) {
            override fun migrate(db: SupportSQLiteDatabase) {
                db.execSQL("ALTER TABLE queue_state ADD COLUMN skipHistory TEXT NOT NULL DEFAULT ''")
                db.execSQL("ALTER TABLE queue_state ADD COLUMN resume TEXT NOT NULL DEFAULT ''")
            }
        }
        fun open(context: Context): LabelingDatabase = Room.databaseBuilder(context.applicationContext,
            LabelingDatabase::class.java, "labeling.db").addMigrations(MIGRATION_1_2,MIGRATION_2_3).build()
    }
}
