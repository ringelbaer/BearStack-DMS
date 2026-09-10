package de.bearstack.people.people

import de.bearstack.people.R
import de.bearstack.people.data.remote.*
import de.bearstack.people.text.*
import kotlinx.coroutines.*
import kotlinx.coroutines.flow.*

data class ManualMergeState(
    val includeNamed: Boolean = false, val pages: Map<Int,List<Person>> = emptyMap(),
    val knownPages: Int = 0, val lastCount: Int = 0, val hasNext: Boolean = true,
    val loading: Boolean = false, val error: UiText? = null,
    val selected: List<Person> = emptyList(), val naming: Boolean = false, val name: String = "",
    val duplicateName: Boolean = false, val epoch: Int = 0,
) {
    val count get() = if(knownPages==0) 0 else (knownPages-1)*20+lastCount
    fun person(index: Int) = pages[index/20]?.getOrNull(index%20)
    val retainedName get() = selected.firstOrNull {it.name.isNotEmpty()}?.name.orEmpty()
}

// Retain three pages around the viewport, plus the bounded selection. Small ID
// checkpoints allow scrolling back indefinitely without retaining every group.
class ManualMergeController(private val scope: CoroutineScope, private val api: LabelingService, private val expectedScope: String) {
    private val mutable=MutableStateFlow(ManualMergeState())
    val state=mutable.asStateFlow()
    private var request: Job?=null
    private var generation=0
    private val ends=ArrayList<Long>()
    private var upper=0L
    private var ready=false
    private var failedPage: Int?=null
    private var visible=0..0
    private fun update(block:(ManualMergeState)->ManualMergeState) = mutable.update(block)

    fun reset(includeNamed: Boolean = state.value.includeNamed) {
        cancel()
        val current= generation
        visible=0..0;upper=0;ready=false;failedPage=null;ends.clear()
        update {ManualMergeState(includeNamed=includeNamed,loading=true,epoch=it.epoch+1)}
        request=scope.launch {
            try {
                val session=api.session()
                currentCoroutineContext().ensureActive()
                if(current!=generation) return@launch
                requireMessage(session.scope==expectedScope,R.string.error_scope_changed)
                requireMessage(session.manualMerge,R.string.people_manual_merge_version)
                upper=session.upper;ready=true
                load(0,current)
            } catch(e:CancellationException) {throw e}
            catch(e:Exception) {if(current==generation) update {it.copy(error=failureText(e))}}
            finally {if(current==generation) update {it.copy(loading=false)}}
        }
    }
    fun visible(first: Int, last: Int) {
        visible=first.coerceAtLeast(0)..last.coerceAtLeast(first).coerceAtLeast(0)
        val s=state.value
        if(s.loading || s.error!=null || s.naming) return
        if(!ready) {reset();return}
        val missing=(visible.first/20..visible.last/20).firstOrNull {it<s.knownPages && it !in s.pages}
        if(missing!=null) requestPage(missing)
        else if(s.hasNext && visible.last>=s.count-8) requestPage(s.knownPages)
    }
    private fun requestPage(page: Int) {
        if(state.value.loading) return
        if(!ready) {reset();return}
        val current=generation
        update {it.copy(loading=true,error=null)}
        request=scope.launch {
            try {load(page,current);failedPage=null}
            catch(e:CancellationException) {throw e}
            catch(e:Exception) {if(current==generation) {failedPage=page;update {it.copy(error=failureText(e))}}}
            finally {if(current==generation) update {it.copy(loading=false)}}
        }
    }
    private suspend fun load(page: Int, current: Int) {
        val before=state.value
        val after=if(page==0) 0L else ends[page-1]
        // Reload only the original ID interval: deleted groups cannot pull a
        // later page's records into this page and create duplicate selections.
        val end=ends.getOrNull(page) ?: upper
        val result=api.mergeGroups(after,end,before.includeNamed)
        currentCoroutineContext().ensureActive()
        if(current!=generation) return
        val newPage=page==ends.size
        if(newPage) ends.add(result.next)
        update {s ->
            val pages=s.pages+ (page to result.people)
            val protected=(visible.first/20..visible.last/20).toSet()+page
            val kept=pages.keys.sortedBy {k -> if(k in protected) -1 else kotlin.math.abs(k-page)}.take(maxOf(3,protected.size)).toSet()
            s.copy(pages=pages.filterKeys {it in kept},knownPages=ends.size,
                lastCount=if(newPage) result.people.size else s.lastCount,
                hasNext=if(newPage) result.hasNext else s.hasNext)
        }
    }
    fun retry() {val page=failedPage;if(page==null) reset() else requestPage(page)}
    fun select(person: Person) {
        update {s ->
            if(s.naming) s else if(s.selected.any {it.id==person.id}) s.copy(selected=s.selected.filterNot {it.id==person.id})
            else if(s.selected.size>=60) s
            else s.copy(selected=s.selected+person)
        }
    }
    fun clearSelection() {update {it.copy(selected=emptyList(),error=null)}}
    fun startNaming() {if(state.value.selected.size>=2) update {it.copy(naming=true,name=it.retainedName,duplicateName=false)}}
    fun closeNaming() {update {it.copy(naming=false,duplicateName=false)}}
    fun nameChanged(name: String) {update {it.copy(name=name,duplicateName=false)}}
    fun duplicateName() {update {it.copy(duplicateName=true)}}
    fun refresh() {
        cancel()
        update {it.copy(pages=emptyMap(),selected=emptyList(),naming=false,error=null,duplicateName=false)}
        requestPage((visible.first/20).coerceAtMost((state.value.knownPages-1).coerceAtLeast(0)))
    }
    fun cancel() {generation++;request?.cancel();request=null;update {it.copy(loading=false)}}
}
