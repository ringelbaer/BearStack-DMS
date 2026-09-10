package de.bearstack.people

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.runtime.LaunchedEffect
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import de.bearstack.people.people.PeopleViewModel
import de.bearstack.people.ui.PeopleApp

class MainActivity : ComponentActivity() {
    private var model: PeopleViewModel? = null
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        setContent {
            val vm: PeopleViewModel = viewModel()
            model=vm
            LaunchedEffect(vm) {
                // Composition can attach after onStart, including with a retained ViewModel.
                if(lifecycle.currentState.isAtLeast(Lifecycle.State.STARTED)) vm.foreground()
            }
            PeopleApp(vm)
        }
    }
    override fun onStart() { super.onStart(); model?.foreground() }
    override fun onStop() { if (!isChangingConfigurations) model?.background(); super.onStop() }
}
