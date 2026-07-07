import React from 'react';
import { View, Text, StyleSheet, TouchableOpacity, Alert } from 'react-native';
import * as SecureStore from 'expo-secure-store';

export default function SettingsScreen({ navigation }: any) {
  const handleLogout = async () => {
    await SecureStore.deleteItemAsync('access_token');
    await SecureStore.deleteItemAsync('refresh_token');
    navigation.replace('Login');
  };

  return (
    <View style={styles.container}>
      <Text style={styles.title}>Настройки</Text>

      <View style={styles.card}>
        <Text style={styles.cardTitle}>О системе</Text>
        <Text style={styles.cardText}>NRV 0.1.0</Text>
        <Text style={styles.cardText}>Промышленный NVR для OpenIPC</Text>
      </View>

      <TouchableOpacity style={styles.logoutButton} onPress={handleLogout}>
        <Text style={styles.logoutText}>Выйти</Text>
      </TouchableOpacity>
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: '#f5f5f5', padding: 16 },
  title: { fontSize: 24, fontWeight: '700', marginBottom: 16 },
  card: {
    backgroundColor: '#fff', borderRadius: 12, padding: 20, marginBottom: 24,
    elevation: 2, shadowColor: '#000', shadowOpacity: 0.05, shadowRadius: 4,
  },
  cardTitle: { fontSize: 16, fontWeight: '600', marginBottom: 8 },
  cardText: { fontSize: 14, color: '#666', marginBottom: 4 },
  logoutButton: {
    backgroundColor: '#ff4d4f', borderRadius: 10, padding: 16, alignItems: 'center',
  },
  logoutText: { color: '#fff', fontSize: 16, fontWeight: '600' },
});