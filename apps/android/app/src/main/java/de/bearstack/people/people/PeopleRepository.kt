package de.bearstack.people.people

import de.bearstack.people.text.*
import de.bearstack.people.R
import androidx.room.withTransaction
import de.bearstack.people.data.local.*
import de.bearstack.people.data.remote.*
import java.util.UUID
import org.json.JSONObject
import org.json.JSONArray

internal fun String.ids(): List<Long> = split(',').mapNotNull { it.toLongOrNull() }
internal fun List<Long>.stored(): String = joinToString(",")
internal data class GroupPosition(val id: Long, val page: Int)
internal fun String.positions(): List<GroupPosition> = if (isEmpty()) emptyList() else split(',').map {
    val parts=it.split(':'); GroupPosition(parts[0].toLong(),parts[1].toInt())
}
internal fun List<GroupPosition>.storedPositions(): String = joinToString(",") { "${it.id}:${it.page}" }
internal fun QueueState.afterReceipt(r: Receipt, merged: Set<Long> = emptySet()): QueueState {
    if(r.action=="merge_groups" || r.action=="name_groups") return copy(
        current=if(current in merged) 0 else current,page=if(current in merged) 0 else page,
        remaining=remaining.ids().filterNot {it in merged}.stored(),
        detached=(detached.ids().filterNot {it in merged}+if(r.action=="merge_groups") listOf(r.target) else emptyList()).distinct().stored(),
        skipped=skipped.ids().filterNot {it in merged}.stored(),
        skipHistory=skipHistory.positions().filterNot {it.id in merged}.storedPositions(),
        resume=resume.positions().filterNot {it.id in merged}.storedPositions(),
        stagedIgnores=stagedIgnores.positions().filterNot {it.id in merged}.storedPositions())
    val next = if (r.action == "detach" || r.action == "unassign") copy(detached=(detached.ids()+r.newId).distinct().stored())
        else if (r.action == "rename" || r.action == "favorite" || r.action == "reject_merge") this
        else if (current==r.source) copy(current=0,page=0) else this
    return if(r.action=="ignore") next.copy(stagedIgnores=next.stagedIgnores.positions().filterNot {it.id==r.source}.storedPositions()) else next
}

class PeopleRepository(private val db: LabelingDatabase, val api: LabelingService, val session: Session) {
    val scope = session.scope
    private val dao = db.dao()
    suspend fun state(): QueueState = dao.state(scope) ?: QueueState(scope, session.upper, UUID.randomUUID().toString()).also { dao.state(it) }
    suspend fun pending(): Pending? = dao.pending(scope)
    suspend fun next(): Person? {
        checkMessage(pending() == null,R.string.error_pending_first)
        var state = state()
        while (true) {
            if (state.current != 0L) {
                try {
                    var person = api.person(state.current, state.page)
                    if (person.name.isEmpty() && person.count > 0) {
                        if (person.faces.isEmpty()) {
                            person = api.person(state.current, ((person.count - 1) / 4 * 4).toInt())
                            state = state.copy(page=person.offset); dao.state(state)
                        }
                        return person
                    }
                } catch (e: ApiFailure) { if (e.status != 404) throw e }
                state = state.copy(current=0,page=0); dao.state(state)
            }
            val detached = state.detached.ids()
            val remaining = state.remaining.ids()
            val resume = state.resume.positions()
            if (detached.isNotEmpty()) {
                state = state.copy(current=detached.first(), detached=detached.drop(1).stored())
            } else if (resume.isNotEmpty()) {
                state = state.copy(current=resume.first().id, page=resume.first().page, resume=resume.drop(1).storedPositions())
            } else if (remaining.isNotEmpty()) {
                state = state.copy(current=remaining.first(), remaining=remaining.drop(1).stored())
            } else if (state.exhausted) return null
            else {
                val page = api.candidates(state.cursor, state.upper)
                val excluded=(state.skipped.ids()+state.stagedIgnores.positions().map {it.id}).toSet()
                state = state.copy(remaining=page.people.map { it.id }.filterNot { it in excluded }.stored(),
                    cursor=page.next, exhausted=!page.hasNext)
            }
            dao.state(state)
        }
    }
    suspend fun page(offset: Int): Person {
        val state = state()
        val p = api.person(state.current, offset)
        dao.state(state.copy(page=offset))
        return p
    }
    suspend fun prepare(person: Person, action: String, name: String = "", target: Person? = null, face: Long = 0,
        allowDuplicate: Boolean = false, favorite: Boolean? = null, suggestionId: Long? = null, assignment: Person? = null) {
        checkMessage(pending() == null,R.string.error_pending_first)
        val operation = UUID.randomUUID().toString()
        val body = JSONObject().put("operation_id",operation).put("dataset",session.dataset).put("revision",person.revision)
            .put("action",action).put("name",name).put("allow_duplicate",allowDuplicate).put("face_id",face)
            .put("target_id",target?.id ?: 0).put("target_revision",target?.revision ?: 0)
            .apply {
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
                } else receipt.action
                val inserted = dao.event(Event(scope,receipt.operation,eventAction,receipt.faces,receipt.groups,receipt.at))
                val groups=JSONObject(pending.body).optJSONArray("groups")
                val merged=if(groups==null) emptySet() else (0 until groups.length()).map {groups.getJSONObject(it).getLong("id")}.toSet()
                if (inserted != -1L) dao.state(state().afterReceipt(receipt,merged))
                dao.clearPending(scope)
            }
            return receipt
        } catch (e: ApiFailure) {
            // Auth/rights/server failures keep the unresolved intent. A definitive rejection does not.
            if (e.status == 400 || e.status == 404 || e.status == 409) dao.clearPending(scope)
            throw e
        }
    }
    suspend fun prepareGroupMerge(groups: List<Person>, name: String? = null, allowDuplicate: Boolean = false) {
        checkMessage(pending()==null,R.string.error_pending_first)
        requireMessage(groups.size in 2..60 && groups.map {it.id}.distinct().size==groups.size,R.string.error_response_invalid)
        val operation=UUID.randomUUID().toString()
        val body=JSONObject().put("operation_id",operation).put("dataset",session.dataset)
            .put("revision",groups.first().revision).put("action",if(name==null) "merge_groups" else "name_groups")
            .put("name",name.orEmpty()).put("allow_duplicate",allowDuplicate)
            .put("groups",JSONArray().apply {groups.forEach {put(JSONObject().put("id",it.id).put("revision",it.revision))}}).toString()
        dao.pending(Pending(scope,operation,groups.first().id,body))
    }
    suspend fun skip(person: Person) = db.withTransaction {
        check(pending() == null)
        val state = state()
        checkMessage(state.current == person.id,R.string.error_group_changed)
        dao.event(Event(scope,"skip:${state.pass}:${person.id}","skip",person.count,1,System.currentTimeMillis()/1000))
        dao.state(state.copy(current=0,page=0,skipped=(state.skipped.ids()+person.id).distinct().stored(),
            skipHistory=(state.skipHistory.positions().filterNot { it.id==person.id }+GroupPosition(person.id,state.page)).storedPositions()))
    }
    suspend fun stageIgnore(person: Person) = db.withTransaction {
        check(pending()==null)
        val before=state()
        check(before.current==person.id)
        dao.state(before.copy(current=0,page=0,
            stagedIgnores=(before.stagedIgnores.positions()+GroupPosition(person.id,before.page)).distinctBy {it.id}.storedPositions()))
    }
    /** Unsent ignores are returned to the queue, never replayed after process death. */
    suspend fun restoreIgnores(ids: Set<Long>? = null, show: Long? = null) = db.withTransaction {
        val before=state()
        val pendingSource=pending()?.source
        val restored=before.stagedIgnores.positions().filter {it.id!=pendingSource && (ids==null || it.id in ids)}
        val selected=restored.firstOrNull {it.id==show}
        val paused=if(selected!=null && before.current!=0L) listOf(GroupPosition(before.current,before.page)) else emptyList()
        dao.state(before.copy(current=selected?.id ?: before.current,page=selected?.page ?: before.page,
            stagedIgnores=before.stagedIgnores.positions().filterNot {it in restored}.storedPositions(),
            resume=(restored.filterNot {it==selected}+paused+before.resume.positions()).distinctBy {it.id}.storedPositions()))
    }
    suspend fun back(): Person? {
        checkMessage(pending() == null,R.string.error_pending_first)
        while (true) {
            val before=state()
            val history=before.skipHistory.positions()
            val previous=history.lastOrNull() ?: return null
            // Validate remotely before changing local queue or statistics. A network
            // failure leaves both the current card and the undo opportunity intact.
            var person=try { api.person(previous.id,previous.page) }
                catch(e: ApiFailure) { if(e.status!=404) throw e; null }
            if(person!=null && person.name.isEmpty() && person.count>0 && person.faces.isEmpty()) {
                person=api.person(previous.id,((person.count-1)/4*4).toInt())
            }
            val usable=person!=null && person.name.isEmpty() && person.count>0
            db.withTransaction {
                checkMessage(pending()==null && state()==before,R.string.error_queue_changed)
                val trimmed=before.copy(skipHistory=history.dropLast(1).storedPositions())
                if(usable) {
                    val resume=(if(before.current!=0L && before.current!=previous.id) listOf(GroupPosition(before.current,before.page)) else emptyList()) + before.resume.positions()
                    dao.state(trimmed.copy(current=previous.id,page=person!!.offset,
                        resume=resume.filterNot { it.id==previous.id }.distinctBy { it.id }.storedPositions(),
                        remaining=before.remaining.ids().filterNot { it==previous.id }.stored(),
                        detached=before.detached.ids().filterNot { it==previous.id }.stored(),
                        skipped=before.skipped.ids().filterNot { it==previous.id }.stored()))
                    dao.undoSkipEvent(scope,"skip:${before.pass}:${previous.id}")
                } else dao.state(trimmed)
            }
            if(usable) return person
        }
    }
    suspend fun newPass(skippedOnly: Boolean) {
        check(pending() == null)
        val old = state()
        val fresh = api.session()
        requireMessage(fresh.scope == scope,R.string.error_scope_changed)
        dao.state(QueueState(scope,fresh.upper,UUID.randomUUID().toString(),
            remaining=if(skippedOnly) old.skipped else "", exhausted=skippedOnly,
            detached=old.detached,skipped=if(skippedOnly) "" else old.skipped,stagedIgnores=old.stagedIgnores))
    }
    fun statistics(today: Long) = dao.statistics(scope,today)
}
