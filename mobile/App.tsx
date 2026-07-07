import React from 'react';
import { StatusBar } from 'expo-status-bar';
import { NavigationContainer } from '@react-navigation/native';
import { createNativeStackNavigator } from '@react-navigation/native-stack';
import { createBottomTabNavigator } from '@react-navigation/bottom-tabs';
import LoginScreen from './src/screens/LoginScreen';
import DashboardScreen from './src/screens/DashboardScreen';
import CamerasScreen from './src/screens/CamerasScreen';
import CameraDetailScreen from './src/screens/CameraDetailScreen';
import RecordingsScreen from './src/screens/RecordingsScreen';
import SettingsScreen from './src/screens/SettingsScreen';

const Stack = createNativeStackNavigator();
const Tab = createBottomTabNavigator();

function MainTabs() {
  return (
    <Tab.Navigator screenOptions={{ headerShown: false }}>
      <Tab.Screen name="DashboardTab" component={DashboardScreen} options={{ tabBarLabel: 'Дашборд' }} />
      <Tab.Screen name="CamerasTab" component={CamerasScreen} options={{ tabBarLabel: 'Камеры' }} />
      <Tab.Screen name="RecordingsTab" component={RecordingsScreen} options={{ tabBarLabel: 'Записи' }} />
      <Tab.Screen name="SettingsTab" component={SettingsScreen} options={{ tabBarLabel: 'Настройки' }} />
    </Tab.Navigator>
  );
}

export default function App() {
  const [token, setToken] = React.useState<string | null>(null);

  // Expo SecureStore для токенов
  React.useEffect(() => {
    // В реальном приложении: SecureStore.getItemAsync('access_token')
  }, []);

  return (
    <NavigationContainer>
      <StatusBar style="auto" />
      <Stack.Navigator screenOptions={{ headerShown: false }}>
        {!token ? (
          <Stack.Screen name="Login" component={LoginScreen} />
        ) : (
          <>
            <Stack.Screen name="Main" component={MainTabs} />
            <Stack.Screen name="CameraDetail" component={CameraDetailScreen} options={{ headerShown: true, title: 'Камера' }} />
          </>
        )}
      </Stack.Navigator>
    </NavigationContainer>
  );
}