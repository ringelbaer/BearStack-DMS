package de.bearstack.people.people

import de.bearstack.people.text.*
import de.bearstack.people.R
import androidx.room.withTransaction
import de.bearstack.people.data.local.*
import de.bearstack.people.data.remote.*
import java.util.UUID
import org.json.JSONObject
import org.json.JSONArray

internal fun QueueState.afterReceipt(r: Receipt, merged: Set<Long> = emptySet()): QueueState {
    val clear = if(r.action=="merge_groups" || r.action=="name_groups") current in merged
        else r.action !in setOf("detach","unassign","favorite","rename","reject_merge") && !r.action.endsWith("_faces") && current==r.source
    return if(clear) copy(current=0,page=0) else this
}

class PeopleRepository(private val db: LabelingDatabase, val api: LabelingService, val session: Session) {
    val scope = session.scope
    private val dao = db.dao()
    suspend fun state(): QueueState = dao.state(scope) ?: run {
        dao.initialize(QueueState(scope,session.upper,UUID.randomUUID().toString()))
        checkNotNull(dao.state(scope))
    }
    suspend fun pending(): Pending? = dao.pending(scope)
    suspend fun queueStatus(): QueueStatus = dao.queueStatus(scope)
    private suspend fun save(value: QueueState) = dao.state(value.copy(revision=value.revision+1))
    private suspend fun append(kind: String, person: Long, page: Int = 0) {
        if(dao.entry(scope,kind,person)!=null) return
        dao.entry(QueueEntry(scope,kind,person,(dao.lastEntry(scope,kind)?.position ?: -1)+1,page))
    }
    private suspend fun prepend(kind: String, person: Long, page: Int = 0) {
        dao.removeEntry(scope,kind,person)
        dao.entry(QueueEntry(scope,kind,person,(dao.firstEntry(scope,kind)?.position ?: 1)-1,page))
    }
    private suspend fun checkUnchanged(before: QueueState) {
        checkMessage(pending()==null && state()==before,R.string.error_queue_changed)
    }
    suspend fun next(): Person? {
        while(true) {
            checkMessage(pending()==null,R.string.error_pending_first)
            val before=state()
            if(before.current!=0L) {
                var person=try { api.person(before.current,before.page) }
                    catch(e: ApiFailure) { if(e.status!=404) throw e; null }
                if(person!=null && person.name.isEmpty() && person.count>0) {
                    if(person.faces.isEmpty()) person=api.person(before.current,((person.count-1)/4*4).toInt())
                    db.withTransaction {
                        checkUnchanged(before)
                        if(person.offset!=before.page) save(before.copy(page=person.offset))
                    }
                    return person
                }
                db.withTransaction { checkUnchanged(before);save(before.copy(current=0,page=0)) }
                continue
            }
            val selected=db.withTransaction {
                checkUnchanged(before)
                val entry=dao.firstEntry(scope,QueueKind.Detached) ?: dao.firstEntry(scope,QueueKind.Resume)
                    ?: dao.firstEntry(scope,QueueKind.Remaining)
                if(entry!=null) {
                    dao.removeEntry(scope,entry.kind,entry.person)
                    save(before.copy(current=entry.person,page=entry.page))
                }
                entry!=null
            }
            if(selected) continue
            if(before.exhausted) return null
            val page=api.candidates(before.cursor,before.upper)
            checkMessage(!page.hasNext || page.next>before.cursor,R.string.error_queue_changed)
            db.withTransaction {
                checkUnchanged(before)
                val excluded=dao.excludedPeople(scope,page.people.map {it.id}).toSet()
                page.people.filterNot {it.id in excluded}.forEach {append(QueueKind.Remaining,it.id)}
                save(before.copy(cursor=page.next,exhausted=!page.hasNext))
            }
        }
    }
    suspend fun page(offset: Int): Person {
        val before=state()
        val person=api.person(before.current,offset)
        db.withTransaction {checkUnchanged(before);save(before.copy(page=offset))}
        return person
    }
    suspend fun prepare(person: Person, action: String, name: String = "", target: Person? = null, face: Long = 0,
        allowDuplicate: Boolean = false, favorite: Boolean? = null, suggestionId: Long? = null, assignment: Person? = null, faces: Set<Long> = emptySet(), directory: String? = null) {
        checkMessage(pending() == null,R.string.error_pending_first)
        val operation = UUID.randomUUID().toString()
        val body = JSONObject().put("operation_id",operation).put("dataset",session.dataset).put("revision",person.revision)
            .put("action",action).put("name",name).put("allow_duplicate",allowDuplicate).put("face_id",face)
            .put("target_id",target?.id ?: 0).put("target_revision",target?.revision ?: 0)
            .apply {
                if(directory!=null) put("directory",directory)
                if(faces.isNotEmpty()) put("face_ids",JSONArray(faces.sorted()))
                if(favorite!=null) put("favorite",favorite)
                if(suggestionId!=null) put("suggestion_id",suggestionId)
                if(assignment!=null) { put("assign_id",assignment.id); put("assign_revision",assignment.revision) }
            }.toString()
        // Saved before transmission. There is at most one unresolved write per scope.
        dao.pending(Pending(scope,operation,person.id,body))
    }
    suspend fun resolve(): Receipt? {
        val pending = pending() ?: return null
        try {
            val receipt = try { api.receipt(pending.operation,session.dataset) }
                catch (e: ApiFailure) { if(e.status != 404) throw e; api.action(pending.source,pending.body) }
            requireMessage(receipt.operation == pending.operation && receipt.source == pending.source,R.string.error_receipt)
            db.withTransaction {
                val eventAction=if(receipt.action=="name_groups") "name" else if(receipt.action=="name_merge") {
                    if(JSONObject(pending.body).optLong("assign_id")!=0L) "assign" else "name"
                } else when(receipt.action) {
                    "name_faces" -> "name"; "assign_faces" -> "assign"; "ignore_faces", "folder_ignore" -> "ignore"
                    "folder_move" -> if(JSONObject(pending.body).optLong("target_id")>0) "assign" else "name"
                    else -> receipt.action
                }
                val inserted = dao.event(Event(scope,receipt.operation,eventAction,receipt.faces,receipt.groups,receipt.at))
                val groups=JSONObject(pending.body).optJSONArray("groups")
                val merged=if(groups==null) emptySet() else (0 until groups.length()).map {groups.getJSONObject(it).getLong("id")}.toSet()
                if (inserted != -1L) applyReceipt(receipt,merged)
                dao.clearPending(scope)
            }
            return receipt
        } catch (e: ApiFailure) {
            // Auth/rights/server failures keep the unresolved intent. A definitive rejection does not.
            if (e.status == 400 || e.status == 404 || e.status == 409) dao.clearPending(scope)
            throw e
        }
    }
    private suspend fun applyReceipt(receipt: Receipt, merged: Set<Long>) {
        val before=state()
        if(receipt.action=="merge_groups" || receipt.action=="name_groups") {
            dao.removePeople(scope,merged.toList())
            if(receipt.action=="merge_groups") append(QueueKind.Detached,receipt.target)
        } else if(receipt.action in setOf("detach","unassign","unassign_faces","folder_unnamed","folder_exclude")) {
            append(QueueKind.Detached,receipt.newId)
        }
        if(receipt.action=="ignore") dao.removeEntry(scope,QueueKind.Staged,receipt.source)
        save(before.afterReceipt(receipt,merged))
    }
    suspend fun skip(person: Person) = db.withTransaction {
        check(pending()==null)
        val before=state()
        checkMessage(before.current==person.id,R.string.error_group_changed)
        dao.event(Event(scope,"skip:${before.pass}:${person.id}","skip",person.count,1,System.currentTimeMillis()/1000))
        append(QueueKind.Skipped,person.id)
        dao.removeEntry(scope,QueueKind.History,person.id)
        append(QueueKind.History,person.id,before.page)
        save(before.copy(current=0,page=0))
    }
    suspend fun stageIgnore(person: Person) = db.withTransaction {
        check(pending()==null)
        val before=state()
        check(before.current==person.id)
        append(QueueKind.Staged,person.id,before.page)
        save(before.copy(current=0,page=0))
    }
    /** Unsent ignores are returned to the queue, never replayed after process death. */
    suspend fun restoreIgnores(ids: Set<Long>? = null, show: Long? = null, except: Set<Long> = emptySet()) = db.withTransaction {
        val before=state()
        val pendingSource=pending()?.source
        fun eligible(entry: QueueEntry) = entry.person!=pendingSource && entry.person !in except && (ids==null || entry.person in ids)
        val selected=show?.let {dao.entry(scope,QueueKind.Staged,it)}?.takeIf {eligible(it)}
        if(selected!=null) {
            dao.removeEntry(scope,QueueKind.Staged,selected.person)
            if(before.current!=0L) prepend(QueueKind.Resume,before.current,before.page)
        }
        // Walk backwards in bounded batches; prepending preserves the original order.
        var cursor=Long.MAX_VALUE
        while(true) {
            val batch=dao.reverseEntries(scope,QueueKind.Staged,cursor)
            if(batch.isEmpty()) break
            batch.filter {eligible(it)}.forEach {
                dao.removeEntry(scope,QueueKind.Staged,it.person)
                prepend(QueueKind.Resume,it.person,it.page)
            }
            cursor=batch.last().position
        }
        save(before.copy(current=selected?.person ?: before.current,page=selected?.page ?: before.page))
    }
    suspend fun back(): Person? {
        checkMessage(pending()==null,R.string.error_pending_first)
        while(true) {
            val before=state()
            val previous=dao.lastEntry(scope,QueueKind.History) ?: return null
            // Validate remotely before changing queue or statistics.
            var person=try {api.person(previous.person,previous.page)}
                catch(e: ApiFailure) {if(e.status!=404) throw e;null}
            if(person!=null && person.name.isEmpty() && person.count>0 && person.faces.isEmpty()) {
                person=api.person(previous.person,((person.count-1)/4*4).toInt())
            }
            val usable=person!=null && person.name.isEmpty() && person.count>0
            db.withTransaction {
                checkUnchanged(before)
                dao.removeEntry(scope,QueueKind.History,previous.person)
                if(usable) {
                    if(before.current!=0L && before.current!=previous.person) prepend(QueueKind.Resume,before.current,before.page)
                    for(kind in listOf(QueueKind.Resume,QueueKind.Remaining,QueueKind.Detached,QueueKind.Skipped)) {
                        dao.removeEntry(scope,kind,previous.person)
                    }
                    save(before.copy(current=previous.person,page=person!!.offset))
                    dao.undoSkipEvent(scope,"skip:${before.pass}:${previous.person}")
                } else save(before)
            }
            if(usable) return person
        }
    }
    suspend fun newPass(skippedOnly: Boolean) {
        check(pending()==null)
        val before=state()
        val fresh=api.session()
        requireMessage(fresh.scope==scope,R.string.error_scope_changed)
        db.withTransaction {
            checkUnchanged(before)
            for(kind in listOf(QueueKind.Remaining,QueueKind.History,QueueKind.Resume)) dao.clearEntries(scope,kind)
            if(skippedOnly) {
                dao.copyEntries(scope,QueueKind.Skipped,QueueKind.Remaining)
                dao.clearEntries(scope,QueueKind.Skipped)
            }
            save(before.copy(upper=fresh.upper,pass=UUID.randomUUID().toString(),current=0,page=0,cursor=0,exhausted=skippedOnly))
        }
    }
    fun statistics(today: Long) = dao.statistics(scope,today)
}
