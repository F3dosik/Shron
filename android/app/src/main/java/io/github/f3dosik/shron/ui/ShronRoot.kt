package io.github.f3dosik.shron.ui

import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.lifecycle.ViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import io.github.f3dosik.shron.ShronApplication
import io.github.f3dosik.shron.ui.account.SettingsScreen
import io.github.f3dosik.shron.ui.account.AccountViewModel
import io.github.f3dosik.shron.ui.account.TransferViewModel
import io.github.f3dosik.shron.ui.auth.AuthViewModel
import io.github.f3dosik.shron.ui.auth.CreateStandaloneScreen
import io.github.f3dosik.shron.ui.common.accountName
import io.github.f3dosik.shron.ui.auth.KdfDowngradeDialog
import io.github.f3dosik.shron.ui.auth.PasswordChangeScreen
import io.github.f3dosik.shron.ui.auth.SignInScreen
import io.github.f3dosik.shron.ui.auth.UnlockScreen
import io.github.f3dosik.shron.ui.secrets.ConflictsScreen
import io.github.f3dosik.shron.ui.secrets.ConflictsViewModel
import io.github.f3dosik.shron.ui.secrets.SecretDetails
import io.github.f3dosik.shron.ui.secrets.SecretFormScreen
import io.github.f3dosik.shron.ui.secrets.SecretFormViewModel
import io.github.f3dosik.shron.ui.secrets.SecretRef
import io.github.f3dosik.shron.ui.secrets.SecretScreen
import io.github.f3dosik.shron.ui.secrets.SecretViewModel
import io.github.f3dosik.shron.ui.secrets.SecretsScreen
import io.github.f3dosik.shron.ui.secrets.SecretsViewModel
import io.github.f3dosik.shron.ui.server.ServerScreen
import io.github.f3dosik.shron.ui.server.ServerViewModel

/** Экраны разблокированного хранилища. */
sealed interface VaultRoute {
    data object List : VaultRoute
    data object Add : VaultRoute
    data class View(val ref: SecretRef) : VaultRoute
    data class Edit(val secret: SecretDetails) : VaultRoute
    data object Settings : VaultRoute
    data object Conflicts : VaultRoute
}

/**
 * Состояние навигации. Хранится во ViewModel, а не в rememberSaveable: названия и
 * содержимое секретов не должны попадать в сохраняемое на диск состояние Activity.
 */
class NavigationViewModel : ViewModel() {
    var route by mutableStateOf<VaultRoute>(VaultRoute.List)
        private set
    var serverSettings by mutableStateOf(false)

    /** Номер открытия экрана: новый ключ — новый ViewModel экрана со свежими данными. */
    var session by mutableIntStateOf(0)
        private set

    fun open(next: VaultRoute) {
        session++
        route = next
    }

    fun openServerSettings() {
        session++
        serverSettings = true
    }
}

/**
 * Экран выбирается по состоянию ядра, а не стеку навигации: блокировка из потока Go
 * (бездействие, истёкшая сессия) сразу возвращает к разблокировке, с какого бы экрана
 * это ни случилось.
 */
@Composable
fun ShronRoot(app: ShronApplication) {
    val state by app.core.state.collectAsStateWithLifecycle()
    val auth: AuthViewModel = viewModel { AuthViewModel(app) }
    val nav: NavigationViewModel = viewModel()
    val session = nav.session

    // Открытый секрет и незаконченная форма не переживают блокировку.
    LaunchedEffect(state.unlocked) {
        if (!state.unlocked) nav.open(VaultRoute.List)
    }

    when {
        !state.configured || nav.serverSettings -> {
            val vm: ServerViewModel = viewModel(key = "server-$session") { ServerViewModel(app) }
            ServerScreen(
                vm = vm,
                onSaved = { nav.serverSettings = false },
                onClose = if (state.configured) ({ nav.serverSettings = false }) else null,
            )
        }

        state.standalone && !state.hasStandaloneVault -> CreateStandaloneScreen(auth, nav::openServerSettings)

        state.login.isEmpty() && !state.standalone -> SignInScreen(auth, state.serverAddress, nav::openServerSettings)

        !state.unlocked && auth.temporaryPassword != null -> PasswordChangeScreen(auth, state.login)

        !state.unlocked -> UnlockScreen(
            auth, accountName(state), state.lockReason, nav::openServerSettings, standalone = state.standalone,
        )

        else -> when (val route = nav.route) {
            VaultRoute.List -> {
                val vm: SecretsViewModel = viewModel { SecretsViewModel(app) }
                SecretsScreen(
                    vm = vm,
                    onAdd = { nav.open(VaultRoute.Add) },
                    onOpen = { nav.open(VaultRoute.View(it)) },
                    onSettings = { nav.open(VaultRoute.Settings) },
                    onConflicts = { nav.open(VaultRoute.Conflicts) },
                )
            }

            VaultRoute.Conflicts -> {
                val vm: ConflictsViewModel = viewModel(key = "conflicts-$session") { ConflictsViewModel(app) }
                ConflictsScreen(vm = vm, onBack = { nav.open(VaultRoute.List) })
            }

            VaultRoute.Add -> {
                val vm: SecretFormViewModel = viewModel(key = "add-$session") { SecretFormViewModel(app) }
                SecretFormScreen(
                    vm = vm,
                    onSaved = { nav.open(VaultRoute.View(it)) },
                    onClose = { nav.open(VaultRoute.List) },
                )
            }

            is VaultRoute.View -> {
                val vm: SecretViewModel = viewModel(key = "view-$session") { SecretViewModel(app, route.ref) }
                SecretScreen(
                    vm = vm,
                    onBack = { nav.open(VaultRoute.List) },
                    onEdit = { nav.open(VaultRoute.Edit(it)) },
                    onDeleted = { nav.open(VaultRoute.List) },
                )
            }

            VaultRoute.Settings -> {
                val vm: AccountViewModel = viewModel(key = "settings-$session") { AccountViewModel(app) }
                val transfer: TransferViewModel = viewModel(key = "transfer-$session") { TransferViewModel(app) }
                SettingsScreen(
                    vm = vm,
                    transfer = transfer,
                    login = state.login,
                    serverAddress = state.serverAddress,
                    standalone = state.standalone,
                    onBack = { nav.open(VaultRoute.List) },
                    onConnection = nav::openServerSettings,
                )
            }

            is VaultRoute.Edit -> {
                val vm: SecretFormViewModel = viewModel(key = "edit-$session") {
                    SecretFormViewModel(app, route.secret)
                }
                SecretFormScreen(
                    vm = vm,
                    onSaved = { nav.open(VaultRoute.View(it)) },
                    onClose = { nav.open(VaultRoute.View(route.secret.ref)) },
                )
            }
        }
    }
    KdfDowngradeDialog(auth)
}
