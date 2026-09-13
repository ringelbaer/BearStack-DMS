package de.bearstack.people.people

import de.bearstack.people.data.remote.FaceMatch
import de.bearstack.people.data.remote.MergeSuggestion
import de.bearstack.people.text.UiText
import de.bearstack.people.text.failureText
import kotlinx.coroutines.*
import kotlinx.coroutines.flow.*

internal data class MergeFaceMatches(val matches: List<FaceMatch> = emptyList(), val loading: Boolean = false,
    val complete: Boolean = false, val error: UiText? = null)

/** One shared ranking for an unnamed pair, using only the first group's witness.
 * Owned by the ViewModel; all operations and state publication run on Main.
 */
internal class MergeFaceSearch(private val scope: CoroutineScope, pair: MergeSuggestion,
    private val search: (Long) -> Flow<List<FaceMatch>>) {
    private val face = (pair.source.faces.firstOrNull() ?: pair.source.faceId)
        .takeIf { it > 0 && pair.source.name.isEmpty() && pair.target.name.isEmpty() }
    private val excluded = setOf(pair.source.id, pair.target.id)
    private val mutable = MutableStateFlow(face?.let { MergeFaceMatches() })
    val state = mutable.asStateFlow()
    private var job: Job? = null
    private var ticket = 0L
    private var active = false
    private var closed = false

    fun resume() {
        if(closed) return
        active = true
        state.value?.let { if(!it.complete && it.error == null && job == null) start() }
    }

    fun pause() {
        active = false
        ticket++
        job?.cancel(); job = null
        // An incomplete stream cannot retain actionable preliminary matches.
        mutable.update { if(it?.loading == true) MergeFaceMatches() else it }
    }

    fun retry() {
        if(active && !closed && state.value != null && job == null) start()
    }

    fun close() { closed = true; pause(); mutable.value = null }

    private fun start() {
        val witness = face ?: return
        val currentTicket = ++ticket
        fun current() = active && !closed && ticket == currentTicket
        mutable.value = MergeFaceMatches(loading=true)
        // Install the job before executing a possibly synchronous cached flow.
        val next = scope.launch(start=CoroutineStart.LAZY) {
            try {
                search(witness).collect { matches ->
                    if(current()) mutable.value = MergeFaceMatches(matches=matches.asSequence()
                        .filter { it.id !in excluded && it.name.isNotBlank() }.take(20).toList(), loading=true)
                }
                if(current()) mutable.update { it?.copy(loading=false, complete=true) }
            } catch(e: CancellationException) { throw e }
            catch(e: Exception) {
                if(current()) mutable.value = MergeFaceMatches(error=failureText(e))
            } finally {
                if(ticket == currentTicket) job = null
            }
        }
        job = next
        next.start()
    }
}
