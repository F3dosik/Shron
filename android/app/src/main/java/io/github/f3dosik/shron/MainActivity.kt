package io.github.f3dosik.shron

import android.os.Bundle
import android.view.WindowManager
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.fragment.app.FragmentActivity
import io.github.f3dosik.shron.ui.ShronRoot
import io.github.f3dosik.shron.ui.theme.ShronTheme

// FragmentActivity, а не ComponentActivity: его требует BiometricPrompt.
class MainActivity : FragmentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        // Секреты не должны попадать на скриншоты, запись экрана и миниатюру в «Недавних».
        window.addFlags(WindowManager.LayoutParams.FLAG_SECURE)
        enableEdgeToEdge()
        val app = application as ShronApplication
        setContent {
            ShronTheme {
                ShronRoot(app)
            }
        }
    }
}
