package de.bearstack.people.people

import android.content.Context
import android.net.ConnectivityManager
import android.net.Network
import android.net.NetworkCapabilities
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.channels.awaitClose
import kotlinx.coroutines.flow.*
import kotlinx.coroutines.sync.Semaphore
import kotlinx.coroutines.sync.withPermit

/** Uses only the default Wi-Fi route; one original request at a time across visible tiles. */
internal class WifiOriginalPreloader(private val wifi: Flow<Boolean>, private val wifiNow: () -> Boolean) {
    constructor(context: Context, scope: CoroutineScope) : this(
        observeWifi(context.getSystemService(ConnectivityManager::class.java),scope),
        { context.getSystemService(ConnectivityManager::class.java).let { isWifi(it.getNetworkCapabilities(it.activeNetwork)) } },
    )
    private val gate = Semaphore(1)

    suspend fun preload(keys: List<String>, load: suspend (String) -> Unit) {
        wifi.collectLatest { available ->
            if (available) keys.distinct().forEach { key ->
                gate.withPermit {
                    // Recheck after waiting for another tile's request.
                    if (wifiNow()) load(key)
                }
            }
        }
    }

    internal companion object {
        private fun observeWifi(connectivity: ConnectivityManager, scope: CoroutineScope) = callbackFlow {
            var current: Network? = null
            val callback = object : ConnectivityManager.NetworkCallback() {
                override fun onAvailable(network: Network) { current=network;trySend(false) }
                override fun onCapabilitiesChanged(network: Network, capabilities: NetworkCapabilities) {
                    if(network == current) trySend(network == connectivity.activeNetwork && isWifi(capabilities))
                }
                override fun onLost(network: Network) { if(network == current) { current=null;trySend(false) } }
            }
            connectivity.registerDefaultNetworkCallback(callback)
            awaitClose { connectivity.unregisterNetworkCallback(callback) }
        }.distinctUntilChanged().shareIn(scope, SharingStarted.WhileSubscribed(replayExpirationMillis = 0), replay = 1)

        fun isWifi(capabilities: NetworkCapabilities?): Boolean =
            capabilities?.hasTransport(NetworkCapabilities.TRANSPORT_WIFI) == true &&
                !capabilities.hasTransport(NetworkCapabilities.TRANSPORT_CELLULAR) &&
                capabilities.hasCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET)
    }
}
