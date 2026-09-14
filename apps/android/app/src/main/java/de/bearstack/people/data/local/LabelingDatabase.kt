package de.bearstack.people.data.local

import android.content.Context
import androidx.room.*
import androidx.room.migration.Migration
import androidx.sqlite.db.SupportSQLiteDatabase
import kotlinx.coroutines.flow.Flow

@Entity(tableName = "queue_state")
data class QueueState(@PrimaryKey val scope: String, val upper: Long, val pass: String,
    val current: Long = 0, val page: Int = 0, val cursor: Long = 0, val exhausted: Boolean = false,
    @ColumnInfo(defaultValue="0") val revision: Long = 0)

object QueueKind {
    const val Remaining = "remaining"
    const val Detached = "detached"
    const val Skipped = "skipped"
    const val History = "history"
    const val Resume = "resume"
    const val Staged = "staged"
}

@Entity(tableName="queue_entries", primaryKeys=["scope","kind","person"],
    indices=[Index(value=["scope","kind","position"])])
data class QueueEntry(val scope: String, val kind: String, val person: Long, val position: Long, val page: Int = 0)
data class QueueStatus(val skipped: Int, val canGoBack: Boolean)
@Entity(tableName = "pending")
data class Pending(@PrimaryKey val scope: String, val operation: String, val source: Long, val body: String)
@Entity(tableName = "events", primaryKeys = ["scope", "operation"], indices = [Index(value=["scope", "at"])])
data class Event(val scope: String, val operation: String, val action: String, val faces: Long, val groups: Int, val at: Long)
@Dao
interface LabelingDao {
    @Query("SELECT * FROM queue_state WHERE scope=:scope") suspend fun state(scope: String): QueueState?
    @Insert(onConflict=OnConflictStrategy.IGNORE) suspend fun initialize(state: QueueState)
    @Upsert suspend fun state(state: QueueState)
    @Query("SELECT * FROM queue_entries WHERE scope=:scope AND kind=:kind ORDER BY position LIMIT 1")
    suspend fun firstEntry(scope: String, kind: String): QueueEntry?
    @Query("SELECT * FROM queue_entries WHERE scope=:scope AND kind=:kind ORDER BY position DESC LIMIT 1")
    suspend fun lastEntry(scope: String, kind: String): QueueEntry?
    @Query("SELECT * FROM queue_entries WHERE scope=:scope AND kind=:kind AND person=:person")
    suspend fun entry(scope: String, kind: String, person: Long): QueueEntry?
    @Query("SELECT * FROM queue_entries WHERE scope=:scope AND kind=:kind AND position<:before ORDER BY position DESC LIMIT 256")
    suspend fun reverseEntries(scope: String, kind: String, before: Long): List<QueueEntry>
    @Insert(onConflict=OnConflictStrategy.ABORT) suspend fun entry(entry: QueueEntry)
    @Query("DELETE FROM queue_entries WHERE scope=:scope AND kind=:kind AND person=:person")
    suspend fun removeEntry(scope: String, kind: String, person: Long)
    @Query("DELETE FROM queue_entries WHERE scope=:scope AND person IN (:people)")
    suspend fun removePeople(scope: String, people: List<Long>)
    @Query("DELETE FROM queue_entries WHERE scope=:scope AND kind=:kind")
    suspend fun clearEntries(scope: String, kind: String)
    @Query("SELECT COUNT(*) FROM queue_entries WHERE scope=:scope AND kind=:kind")
    suspend fun entryCount(scope: String, kind: String): Int
    @Query("SELECT person FROM queue_entries WHERE scope=:scope AND kind IN ('skipped','staged') AND person IN (:people)")
    suspend fun excludedPeople(scope: String, people: List<Long>): List<Long>
    @Query("INSERT INTO queue_entries(scope,kind,person,position,page) SELECT scope,:target,person,position,0 FROM queue_entries WHERE scope=:scope AND kind=:source")
    suspend fun copyEntries(scope: String, source: String, target: String)
    @Query("SELECT (SELECT COUNT(*) FROM queue_entries WHERE scope=:scope AND kind='skipped') AS skipped, EXISTS(SELECT 1 FROM queue_entries WHERE scope=:scope AND kind='history') AS canGoBack")
    suspend fun queueStatus(scope: String): QueueStatus
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
@Database(entities=[QueueState::class, QueueEntry::class, Pending::class, Event::class], version=4, exportSchema=true)
abstract class LabelingDatabase : RoomDatabase() {
    abstract fun dao(): LabelingDao
    companion object {
        val MIGRATION_3_4: Migration = QueueEntriesMigration
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
            LabelingDatabase::class.java, "labeling.db").addMigrations(MIGRATION_1_2,MIGRATION_2_3,MIGRATION_3_4).build()
    }
}
