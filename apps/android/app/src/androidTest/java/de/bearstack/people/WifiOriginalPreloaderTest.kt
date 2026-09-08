package de.bearstack.people

import android.net.ConnectivityManager
import androidx.test.platform.app.InstrumentationRegistry
import org.junit.Assume.assumeTrue

import de.bearstack.people.people.WifiOriginalPreloader
import kotlinx.coroutines.*
import kotlinx.coroutines.flow.MutableStateFlow
import org.junit.Assert.*
import org.junit.Test

class WifiOriginalPreloaderTest {
    @Test fun defaultNetworkCallbackAlsoServesLaterVisibleTiles() = runBlocking {
        val context=InstrumentationRegistry.getInstrumentation().targetContext
        val cm=context.getSystemService(ConnectivityManager::class.java)
        assumeTrue("Requires active Wi-Fi",WifiOriginalPreloader.isWifi(cm.getNetworkCapabilities(cm.activeNetwork)))
        val scope=CoroutineScope(SupervisorJob()+Dispatchers.Default)
        val preloader=WifiOriginalPreloader(context,scope)
        try {
            val first=CompletableDeferred<Unit>()
            val a=launch {preloader.preload(listOf("first")) {first.complete(Unit)}}
            withTimeout(5000){first.await()}
            val second=CompletableDeferred<Unit>()
            val b=launch {preloader.preload(listOf("second")) {second.complete(Unit)}}
            withTimeout(5000){second.await()}
            a.cancelAndJoin();b.cancelAndJoin()
        } finally {scope.cancel()}
    }

    @Test fun wifiLossCancelsRequestAndVisibleTilesShareOneSlot() = runBlocking {
        val wifi=MutableStateFlow(false)
        val preloader=WifiOriginalPreloader(wifi) {wifi.value}
        val started=CompletableDeferred<Unit>()
        val cancelled=CompletableDeferred<Unit>()
        var active=0
        var maxActive=0
        val jobs=(1..3).map { tile -> launch {
            preloader.preload(listOf("$tile")) {
                active++;maxActive=maxOf(maxActive,active);started.complete(Unit)
                try {awaitCancellation()} finally {active--;cancelled.complete(Unit)}
            }
        } }
        try {
            delay(50);assertFalse(started.isCompleted)
            wifi.value=true
            withTimeout(2000){started.await()}
            wifi.value=false
            withTimeout(2000){cancelled.await()}
            jobs.forEach {it.cancelAndJoin()}
            assertEquals(0,active);assertEquals(1,maxActive)
        } finally {jobs.forEach {it.cancelAndJoin()}}
    }

    @Test fun repeatedPhotoKeysLoadOnceAndDisposedTileStops() = runBlocking {
        val wifi=MutableStateFlow(true)
        val preloader=WifiOriginalPreloader(wifi) {true}
        val loaded=mutableListOf<String>()
        val done=CompletableDeferred<Unit>()
        val job=launch {preloader.preload(listOf("photo-a","photo-a","photo-b")) {
            loaded+=it
            if(it=="photo-b") done.complete(Unit)
        }}
        withTimeout(2000){done.await()}
        job.cancelAndJoin()
        wifi.value=false;wifi.value=true
        assertEquals(listOf("photo-a","photo-b"),loaded)
    }
}
