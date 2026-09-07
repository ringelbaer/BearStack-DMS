package de.bearstack.people

import android.os.Bundle
import android.view.WindowManager
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.lifecycle.viewmodel.compose.viewModel
import de.bearstack.people.people.PeopleViewModel
import de.bearstack.people.ui.PeopleApp

class MainActivity : ComponentActivity() {
    private var model: PeopleViewModel? = null
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        // Face images and password fields must not appear in screenshots or the recent-apps preview.
        window.addFlags(WindowManager.LayoutParams.FLAG_SECURE)
        setContent { val vm: PeopleViewModel = viewModel(); model=vm; PeopleApp(vm) }
    }
    override fun onStop() { if (!isChangingConfigurations) model?.background(); super.onStop() }
}
