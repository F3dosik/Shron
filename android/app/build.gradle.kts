plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.compose)
}

android {
    namespace = "io.github.f3dosik.shron"
    compileSdk {
        version = release(37)
    }

    defaultConfig {
        applicationId = "io.github.f3dosik.shron"
        minSdk = 24
        targetSdk = 37
        // Версию задаёт CI из тега релиза (vX.Y.Z → -Pshron.versionName=X.Y.Z и
        // versionCode = X·10000 + Y·100 + Z); локальные сборки — «dev».
        versionCode = providers.gradleProperty("shron.versionCode").orNull?.toInt() ?: 1
        versionName = providers.gradleProperty("shron.versionName").orNull ?: "dev"

        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
        // Каждый инструментальный тест — в новом процессе с пустыми данными приложения:
        // Go-ядро одно на процесс и хранит настройки и сессию в filesDir.
        testInstrumentationRunnerArguments["clearPackageData"] = "true"

        // Go-ядро собирается только для этих ABI (make android-aar): без фильтра APK ставился бы
        // и на 32-битные устройства, где нет libgojni.so, и падал при запуске.
        ndk {
            abiFilters += listOf("arm64-v8a", "x86_64")
        }

        // Соединение без TLS — только для разработки: ./gradlew -Pshron.insecure=true ...
        val insecure = providers.gradleProperty("shron.insecure").orNull == "true"
        buildConfigField("boolean", "SHRON_INSECURE", insecure.toString())
    }

    // Release, подписанный отладочным ключом, — только для smoke-теста в CI
    // (-Pshron.debugSignedRelease=true): настоящие релизы так не собираются.
    val debugSignedRelease = providers.gradleProperty("shron.debugSignedRelease").orNull == "true"

    // Ключ релизов — из переменных окружения: в CI из секретов, локально — путь к своему
    // хранилищу ключей. Без них release-сборка остаётся неподписанной.
    val releaseKeystore = System.getenv("SHRON_KEYSTORE")?.takeIf { it.isNotBlank() }
    signingConfigs {
        if (releaseKeystore != null) {
            create("release") {
                storeFile = file(releaseKeystore)
                storePassword = System.getenv("SHRON_KEYSTORE_PASSWORD")
                keyAlias = System.getenv("SHRON_KEY_ALIAS") ?: "shron"
                // В PKCS12 пароль ключа совпадает с паролем хранилища.
                keyPassword = System.getenv("SHRON_KEYSTORE_PASSWORD")
            }
        }
    }

    buildTypes {
        release {
            signingConfig = when {
                debugSignedRelease -> signingConfigs.getByName("debug")
                releaseKeystore != null -> signingConfigs.getByName("release")
                else -> null
            }
            // R8 для всего приложения. Ограничение packageScope из шаблона Studio ломало
            // запуск: классы Kotlin внутри области переименовывались и закрывались, а код
            // приложения вне её ссылался на них (IllegalAccessError в ShronApplication).
            // Классы gomobile защищены правилами в src/main/keepRules.
            optimization {
                enable = true
            }
        }
    }
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_11
        targetCompatibility = JavaVersion.VERSION_11
    }
    testOptions {
        execution = "ANDROIDX_TEST_ORCHESTRATOR"
    }
    buildFeatures {
        compose = true
        buildConfig = true
    }
}

dependencies {
    // Go-ядро клиента (gomobile): собирается `make android-aar` из корня репозитория.
    implementation(files("libs/shron.aar"))
    implementation(platform(libs.androidx.compose.bom))
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.biometric)
    // biometric 1.1.0 тянет старый fragment; FragmentActivity нужен BiometricPrompt.
    implementation(libs.androidx.fragment.ktx)
    implementation(libs.androidx.compose.material3)
    implementation(libs.androidx.compose.material.icons.core)
    implementation(libs.androidx.compose.ui)
    implementation(libs.androidx.compose.ui.graphics)
    implementation(libs.androidx.compose.ui.tooling.preview)
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.lifecycle.runtime.ktx)
    implementation(libs.androidx.lifecycle.runtime.compose)
    implementation(libs.androidx.lifecycle.process)
    implementation(libs.androidx.lifecycle.viewmodel.compose)
    testImplementation(libs.junit)
    // В android.jar для JVM-тестов org.json — заглушки; настоящая реализация для ShronError.
    testImplementation(libs.org.json)
    androidTestImplementation(platform(libs.androidx.compose.bom))
    androidTestImplementation(libs.androidx.compose.ui.test.junit4)
    androidTestImplementation(libs.androidx.espresso.core)
    androidTestImplementation(libs.androidx.junit)
    androidTestImplementation(libs.androidx.test.runner)
    androidTestUtil(libs.androidx.test.orchestrator)
    debugImplementation(libs.androidx.compose.ui.test.manifest)
    debugImplementation(libs.androidx.compose.ui.tooling)
}