from launch import LaunchDescription
from launch.actions import DeclareLaunchArgument
from launch.substitutions import LaunchConfiguration
from launch_ros.actions import Node


def generate_launch_description():
    return LaunchDescription([
        DeclareLaunchArgument(
            'backend_url', default_value='ws://localhost:8080/ws/bridge',
            description='Go backend WebSocket bridge endpoint (bridge_client --backend-url).',
        ),
        DeclareLaunchArgument(
            'rosbridge_url', default_value='ws://localhost:9090',
            description='rosbridge_server WebSocket endpoint (bridge_client --rosbridge-url).',
        ),
        DeclareLaunchArgument(
            'reconnect_delay', default_value='5.0',
            description='Seconds between Go backend reconnect attempts (bridge_client --reconnect-delay).',
        ),
        DeclareLaunchArgument(
            'object_list_refresh_interval', default_value='10.0',
            description='Seconds between workspace object list refreshes (bridge_client --object-list-refresh-interval).',
        ),
        Node(
            package='rosbridge_server',
            executable='rosbridge_websocket',
            name='rosbridge_websocket',
            output='screen',
            parameters=[
                {'port': 9090}
            ]
        ),
        # WebSocket client that connects to the Go backend (ws://localhost:8080/ws/bridge)
        # and translates action recipes into ROS2 service calls via the rosbridge_websocket
        # server above. See custom_bridge_pkg/bridge_client.py.
        Node(
            package='custom_bridge_pkg',
            executable='bridge_client',
            name='bridge_client',
            output='screen',
            arguments=[
                '--backend-url', LaunchConfiguration('backend_url'),
                '--rosbridge-url', LaunchConfiguration('rosbridge_url'),
                '--reconnect-delay', LaunchConfiguration('reconnect_delay'),
                '--object-list-refresh-interval', LaunchConfiguration('object_list_refresh_interval'),
            ],
        )
    ])
