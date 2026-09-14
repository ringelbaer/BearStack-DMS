package de.bearstack.people.data.local

import androidx.room.migration.Migration
import androidx.sqlite.db.SupportSQLiteDatabase

/** Room runs this migration transactionally; receipts and statistics are untouched. */
internal object QueueEntriesMigration : Migration(3,4) {
    override fun migrate(db: SupportSQLiteDatabase) {
        db.execSQL("CREATE TABLE queue_entries (scope TEXT NOT NULL, kind TEXT NOT NULL, person INTEGER NOT NULL, position INTEGER NOT NULL, page INTEGER NOT NULL, PRIMARY KEY(scope,kind,person))")
        db.execSQL("CREATE INDEX index_queue_entries_scope_kind_position ON queue_entries(scope,kind,position)")
        db.compileStatement("INSERT OR IGNORE INTO queue_entries(scope,kind,person,position,page) VALUES(?,?,?,?,?)").use { insert ->
            db.query("SELECT scope,remaining,detached,skipped,skipHistory,resume,stagedIgnores FROM queue_state").use { rows ->
                val kinds=listOf(QueueKind.Remaining,QueueKind.Detached,QueueKind.Skipped,QueueKind.History,QueueKind.Resume,QueueKind.Staged)
                while(rows.moveToNext()) {
                    val scope=rows.getString(0)
                    kinds.forEachIndexed { i,kind ->
                        // Parse one entry at a time, retaining order without allocating a list.
                        val raw=rows.getString(i+1)
                        var start=0;var position=0L
                        while(start<raw.length) {
                            val end=raw.indexOf(',',start).let {if(it<0) raw.length else it}
                            val token=raw.substring(start,end)
                            val separator=token.indexOf(':')
                            val person=(if(separator<0) token else token.substring(0,separator)).toLongOrNull()
                            val page=if(separator<0) 0 else token.substring(separator+1).toIntOrNull()
                            check(person!=null && person>0 && page!=null && page>=0) {"Invalid legacy queue entry"}
                            insert.bindString(1,scope);insert.bindString(2,kind);insert.bindLong(3,person)
                            insert.bindLong(4,position++);insert.bindLong(5,page.toLong());insert.executeInsert()
                            start=end+1
                        }
                    }
                }
            }
        }
        db.execSQL("CREATE TABLE queue_state_new (scope TEXT NOT NULL PRIMARY KEY, upper INTEGER NOT NULL, pass TEXT NOT NULL, current INTEGER NOT NULL, page INTEGER NOT NULL, cursor INTEGER NOT NULL, exhausted INTEGER NOT NULL, revision INTEGER NOT NULL DEFAULT 0)")
        db.execSQL("INSERT INTO queue_state_new(scope,upper,pass,current,page,cursor,exhausted) SELECT scope,upper,pass,current,page,cursor,exhausted FROM queue_state")
        db.execSQL("DROP TABLE queue_state")
        db.execSQL("ALTER TABLE queue_state_new RENAME TO queue_state")
    }
}
