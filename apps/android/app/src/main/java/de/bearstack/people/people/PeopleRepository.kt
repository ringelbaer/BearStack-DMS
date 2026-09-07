package de.bearstack.people.people

import androidx.room.withTransaction
import de.bearstack.people.data.local.*
import de.bearstack.people.data.remote.*
import java.util.UUID
import org.json.JSONObject

internal fun String.ids(): List<Long> = split(',').mapNotNull { it.toLongOrNull() }
internal fun List<Long>.stored(): String = joinToString(",")
internal fun QueueState.afterReceipt(r: Receipt): QueueState = if (r.action == "detach") {
    copy(detached = (detached.ids() + r.newId).distinct().stored())
} else copy(current=0, page=0)

class PeopleRepository(private val db: LabelingDatabase, val api: LabelingService, val session: Session) {
    val scope = session.scope
    private val dao = db.dao()
    suspend fun state(): QueueState = dao.state(scope) ?: QueueState(scope, session.upper, UUID.randomUUID().toString()).also { dao.state(it) }
    suspend fun pending(): Pending? = dao.pending(scope)
    suspend fun next(): Person? {
        check(pending() == null) { "Zuerst die offene Aktion klären." }
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
            if (detached.isNotEmpty()) {
                state = state.copy(current=detached.first(), detached=detached.drop(1).stored())
            } else if (remaining.isNotEmpty()) {
                state = state.copy(current=remaining.first(), remaining=remaining.drop(1).stored())
            } else if (state.exhausted) return null
            else {
                val page = api.candidates(state.cursor, state.upper)
                state = state.copy(remaining=page.people.map { it.id }.filterNot { it in state.skipped.ids() }.stored(),
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
        allowDuplicate: Boolean = false) {
        val operation = UUID.randomUUID().toString()
        val body = JSONObject().put("operation_id",operation).put("dataset",session.dataset).put("revision",person.revision)
            .put("action",action).put("name",name).put("allow_duplicate",allowDuplicate).put("face_id",face)
            .put("target_id",target?.id ?: 0).put("target_revision",target?.revision ?: 0).toString()
        // Saved before transmission. There is at most one unresolved write per scope.
        dao.pending(Pending(scope,operation,person.id,body))
    }
    suspend fun resolve(): Receipt? {
        val pending = pending() ?: return null
        try {
            val receipt = try { api.receipt(pending.operation,session.dataset) }
                catch (e: ApiFailure) { if(e.status != 404) throw e; api.action(pending.source,pending.body) }
            require(receipt.operation == pending.operation && receipt.source == pending.source) { "Ungültige Aktionsquittung" }
            db.withTransaction {
                val inserted = dao.event(Event(scope,receipt.operation,receipt.action,receipt.faces,receipt.groups,receipt.at))
                if (inserted != -1L) dao.state(state().afterReceipt(receipt))
                dao.clearPending(scope)
            }
            return receipt
        } catch (e: ApiFailure) {
            // Auth/rights/server failures keep the unresolved intent. A definitive rejection does not.
            if (e.status == 400 || e.status == 404 || e.status == 409) dao.clearPending(scope)
            throw e
        }
    }
    suspend fun skip(person: Person) = db.withTransaction {
        check(pending() == null)
        val state = state()
        dao.event(Event(scope,"skip:${state.pass}:${person.id}","skip",person.count,1,System.currentTimeMillis()/1000))
        dao.state(state.copy(current=0,page=0,skipped=(state.skipped.ids()+person.id).distinct().stored()))
    }
    suspend fun newPass(skippedOnly: Boolean) {
        check(pending() == null)
        val old = state()
        val fresh = api.session()
        require(fresh.scope == scope) { "Datenbestand oder Konto wurde geändert. Verbindung erneut öffnen." }
        dao.state(QueueState(scope,fresh.upper,UUID.randomUUID().toString(),
            remaining=if(skippedOnly) old.skipped else "", exhausted=skippedOnly,
            detached=old.detached,skipped=if(skippedOnly) "" else old.skipped))
    }
    fun statistics(today: Long) = dao.statistics(scope,today)
}
